CREATE TABLE ticket_cancellation_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id UUID NOT NULL REFERENCES tickets(id),
    user_id UUID NOT NULL REFERENCES users(id),
    event_id UUID NOT NULL REFERENCES events(id),
    cancelled_at TIMESTAMP DEFAULT NOW(),
    reason TEXT
);
