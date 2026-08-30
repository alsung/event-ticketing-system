package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/alsung/event-ticketing-system/services/pkg/database"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

// ConsumerGroup is the group id the notification worker joins. Kafka assigns
// each partition to one member of a group, so scaling the worker means running
// more copies with the same id -- no coordination code of our own.
const ConsumerGroup = "notification-worker"

// NotificationConsumer turns ticket events into notification rows.
type NotificationConsumer struct {
	reader *kafka.Reader
}

func NewNotificationConsumer() *NotificationConsumer {
	return &NotificationConsumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers: strings.Split(brokerList(), ","),
			GroupID: ConsumerGroup,
			// One reader over both topics, since the handling differs only in
			// the message body.
			GroupTopics: []string{TopicTicketPurchased, TopicTicketCancelled},
			MinBytes:    1,
			MaxBytes:    10e6,
			MaxWait:     500 * time.Millisecond,
		}),
	}
}

// Run consumes until the context is cancelled.
//
// FetchMessage plus CommitMessages rather than ReadMessage: the offset is
// committed only after the work is durable. Committing on read would lose an
// event if this process died mid-handling.
func (c *NotificationConsumer) Run(ctx context.Context) {
	log.Printf("notification worker joined group %q on %s", ConsumerGroup, brokerList())
	defer func() { _ = c.reader.Close() }()

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Println("notification worker stopping")
				return
			}
			log.Printf("notification worker: fetch: %v", err)
			time.Sleep(time.Second)
			continue
		}

		if err := c.handle(ctx, msg); err != nil {
			// Not committed, so the message is redelivered. A poison message
			// would loop here; a dead-letter topic is the standard answer and
			// is out of scope for this phase.
			log.Printf("notification worker: handling %s: %v", msg.Topic, err)
			continue
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			log.Printf("notification worker: commit: %v", err)
		}
	}
}

func (c *NotificationConsumer) handle(ctx context.Context, msg kafka.Message) error {
	var (
		messageID        uuid.UUID
		ticketID, userID uuid.UUID
		body             string
	)

	switch msg.Topic {
	case TopicTicketPurchased:
		var ev TicketPurchased
		if err := json.Unmarshal(msg.Value, &ev); err != nil {
			return fmt.Errorf("unmarshal: %w", err)
		}
		messageID, ticketID, userID = ev.MessageID, ev.TicketID, ev.UserID
		body = fmt.Sprintf("Your ticket is confirmed. Amount charged: %d cents.", ev.AmountCents)

	case TopicTicketCancelled:
		var ev TicketCancelled
		if err := json.Unmarshal(msg.Value, &ev); err != nil {
			return fmt.Errorf("unmarshal: %w", err)
		}
		messageID, ticketID, userID = ev.MessageID, ev.TicketID, ev.UserID
		body = "Your ticket was cancelled and a refund is on its way."

	default:
		return fmt.Errorf("unknown topic %q", msg.Topic)
	}

	pool, err := database.Pool(ctx)
	if err != nil {
		return err
	}

	// ON CONFLICT DO NOTHING against the unique message_id is what makes this
	// consumer idempotent. Delivery is at-least-once, so a duplicate is expected
	// rather than exceptional. Deduplicating on the message and not on the
	// ticket matters: a cancelled seat returns to the pool and can genuinely be
	// bought again, and that buyer must still get a confirmation.
	_, err = pool.Exec(ctx, `
		INSERT INTO notifications (message_id, ticket_id, user_id, event_type, body)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (message_id) DO NOTHING
	`, messageID, ticketID, userID, msg.Topic, body)
	return err
}
