-- What the consumer produced. This table is the visible proof that the async
-- path ran: a purchase writes an outbox row, the relay publishes it, the worker
-- consumes it and lands a row here.

CREATE TABLE IF NOT EXISTS notifications (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id  UUID NOT NULL,
    user_id    UUID NOT NULL,
    event_type TEXT NOT NULL,
    channel    TEXT NOT NULL DEFAULT 'log',
    body       TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- This is what makes the consumer idempotent. Delivery is at-least-once, so
    -- the same message can arrive twice; the second insert conflicts and is
    -- discarded rather than sending a duplicate confirmation.
    UNIQUE (ticket_id, event_type)
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_id ON notifications (user_id);
