package outbox

import (
	"context"
	"errors"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

// EnsureTopics creates the topics this system uses, if they do not exist.
//
// Auto-creation is enabled on the broker, but it only fires when something
// produces. A consumer group that joins before the first produce is assigned no
// partitions and then sits idle -- it does not retroactively pick the topic up.
// Creating the topics explicitly at startup removes that ordering dependency,
// and makes partition count a decision rather than a broker default.
func EnsureTopics(ctx context.Context) error {
	brokers := strings.Split(brokerList(), ",")

	// The controller is the only broker that accepts topic creation.
	conn, err := kafka.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}

	ctrlConn, err := kafka.DialContext(ctx, "tcp",
		net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return err
	}
	defer func() { _ = ctrlConn.Close() }()

	configs := []kafka.TopicConfig{
		// Three partitions so the consumer group has something to spread across
		// when it is scaled past one member. Ordering is preserved per ticket
		// regardless, because messages are keyed by ticket id.
		{Topic: TopicTicketPurchased, NumPartitions: 3, ReplicationFactor: 1},
		{Topic: TopicTicketCancelled, NumPartitions: 3, ReplicationFactor: 1},
	}

	if err := ctrlConn.CreateTopics(configs...); err != nil {
		return err
	}
	log.Printf("kafka: topics ready (%s, %s)", TopicTicketPurchased, TopicTicketCancelled)
	return nil
}

// WaitForTopics retries EnsureTopics until the broker answers or ctx ends.
// Compose gates on the broker's healthcheck, but a broker that passes its
// health probe can still be a moment away from serving metadata.
func WaitForTopics(ctx context.Context) error {
	var lastErr error
	for attempt := 0; attempt < 15; attempt++ {
		if err := EnsureTopics(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return errors.New("kafka: topics not ready: " + lastErr.Error())
}
