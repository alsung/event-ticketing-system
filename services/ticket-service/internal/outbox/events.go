// Package outbox implements the transactional outbox: events are written to a
// table inside the same transaction as the state change they describe, and a
// relay publishes them to Kafka afterwards.
package outbox

import (
	"time"

	"github.com/google/uuid"
)

// Topics. Both are keyed by ticket id, so every event for one ticket lands on
// the same partition and is consumed in order -- a cancellation can never be
// processed before the purchase it refers to.
const (
	TopicTicketPurchased = "ticket.purchased"
	TopicTicketCancelled = "ticket.cancelled"
)

// TicketPurchased is published when a purchase transaction commits.
type TicketPurchased struct {
	// MessageID identifies this event, not the ticket. Delivery is
	// at-least-once, so consumers deduplicate on this; deduplicating on the
	// ticket would discard a genuine second purchase of a seat that was
	// cancelled and returned to the pool.
	MessageID   uuid.UUID `json:"message_id"`
	EventType   string    `json:"event_type"`
	TicketID    uuid.UUID `json:"ticket_id"`
	EventID     uuid.UUID `json:"event_id"`
	UserID      uuid.UUID `json:"user_id"`
	AmountCents int64     `json:"amount_cents"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// TicketCancelled is published when a cancellation transaction commits.
type TicketCancelled struct {
	MessageID  uuid.UUID `json:"message_id"`
	EventType  string    `json:"event_type"`
	TicketID   uuid.UUID `json:"ticket_id"`
	EventID    uuid.UUID `json:"event_id"`
	UserID     uuid.UUID `json:"user_id"`
	Reason     string    `json:"reason,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}
