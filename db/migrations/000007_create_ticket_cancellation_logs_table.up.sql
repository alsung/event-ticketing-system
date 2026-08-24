-- Audit trail for cancellations. Cancelling returns the ticket row to
-- 'available', so the ticket itself retains no history -- this table is the
-- only record that the cancellation happened.
CREATE TABLE IF NOT EXISTS ticket_cancellation_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id UUID NOT NULL REFERENCES tickets(id),
    user_id UUID NOT NULL REFERENCES users(id),
    event_id UUID NOT NULL REFERENCES events(id),
    cancelled_at TIMESTAMP DEFAULT NOW(),
    reason TEXT
);

CREATE INDEX IF NOT EXISTS idx_cancellation_logs_user_cancelled_at
    ON ticket_cancellation_logs (user_id, cancelled_at);
CREATE INDEX IF NOT EXISTS idx_cancellation_logs_ticket_id
    ON ticket_cancellation_logs (ticket_id);
