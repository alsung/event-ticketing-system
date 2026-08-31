package outbox

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/alsung/event-ticketing-system/services/pkg/database"
	"github.com/jackc/pgx/v5"
	"github.com/segmentio/kafka-go"
)

// Relay polls the outbox and publishes to Kafka.
//
// It runs in-process alongside the HTTP server rather than as its own service:
// it is an implementation detail of this service's own writes, and it shares the
// transaction boundary conceptually. The consumer, by contrast, is a separate
// binary, because an independent consumer group is a real deployment boundary.
type Relay struct {
	writer    *kafka.Writer
	interval  time.Duration
	batchSize int
}

func NewRelay() *Relay {
	brokers := strings.Split(brokerList(), ",")
	return &Relay{
		writer: &kafka.Writer{
			Addr: kafka.TCP(brokers...),
			// Topic is set per message, since one relay serves several.
			Balancer: &kafka.Hash{},
			// RequireAll: the broker acknowledges only once the write is on
			// every in-sync replica. Anything weaker means an acknowledged event
			// can still be lost, which defeats the point of the outbox.
			RequiredAcks: kafka.RequireAll,
			Async:        false,
			BatchTimeout: 50 * time.Millisecond,
			// Bounded so a broker outage cannot wedge the relay. kafka-go
			// retries internally with backoff, which without a ceiling means a
			// single WriteMessages call can block for minutes.
			WriteTimeout: 5 * time.Second,
			MaxAttempts:  3,
		},
		interval:  time.Second,
		batchSize: 100,
	}
}

func brokerList() string {
	if v := os.Getenv("KAFKA_BROKERS"); v != "" {
		return v
	}
	return "localhost:9092"
}

// Run polls until the context is cancelled.
func (r *Relay) Run(ctx context.Context) {
	log.Printf("outbox relay started, publishing to %s", brokerList())
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	defer func() { _ = r.writer.Close() }()

	for {
		select {
		case <-ctx.Done():
			log.Println("outbox relay stopping")
			return
		case <-ticker.C:
			if n, err := r.publishBatch(ctx); err != nil {
				log.Printf("outbox relay: %v", err)
			} else if n > 0 {
				log.Printf("outbox relay: published %d event(s)", n)
			}
		}
	}
}

// publishBatch claims a batch, produces it, then records the outcome.
//
// Three separate steps on purpose, with no database transaction held across the
// Kafka write. An earlier version wrapped all three in one transaction so the
// claim and the mark were atomic. That was wrong twice over: it pinned a
// database connection for the length of broker latency, and when a publish
// failed, MarkFailed ran on the pool and blocked forever on the FOR UPDATE
// locks the enclosing transaction still held -- a self-deadlock.
//
// Committing the fetch first means another relay could claim the same rows and
// publish them twice. That is acceptable: delivery is at-least-once by design
// and consumers are idempotent. Losing an event would be the serious failure;
// sending one twice is not.
func (r *Relay) publishBatch(ctx context.Context) (int, error) {
	pool, err := database.Pool(ctx)
	if err != nil {
		return 0, err
	}

	// Step 1: claim a batch and release the locks immediately.
	var records []Record
	err = database.InTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		records, err = FetchUnpublished(ctx, tx, r.batchSize)
		return err
	})
	if err != nil || len(records) == 0 {
		return 0, err
	}

	msgs := make([]kafka.Message, 0, len(records))
	ids := make([]int64, 0, len(records))
	for _, rec := range records {
		msgs = append(msgs, kafka.Message{
			Topic: rec.Topic,
			// Keying by aggregate puts every event for one ticket on the same
			// partition, so ordering per ticket is preserved.
			Key:   []byte(rec.AggregateID.String()),
			Value: rec.Payload,
		})
		ids = append(ids, rec.ID)
	}

	// Step 2: produce, holding no transaction and no lock.
	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := r.writer.WriteMessages(writeCtx, msgs...); err != nil {
		if mErr := MarkFailed(ctx, pool, ids, err.Error()); mErr != nil {
			log.Printf("outbox relay: recording failure: %v", mErr)
		}
		return 0, err
	}

	// Step 3: mark them published. A crash before this point republishes on the
	// next poll, which is the at-least-once guarantee working as intended.
	err = database.InTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		return MarkPublished(ctx, tx, ids)
	})
	if err != nil {
		return 0, err
	}
	return len(records), nil
}
