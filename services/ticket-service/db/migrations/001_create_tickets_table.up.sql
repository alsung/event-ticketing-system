CREATE TABLE tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(id),
    user_id UUID REFERENCES users(id),
    price DECIMAL(10, 2) NOT NULL,
    status TEXT NOT NULL DEFAULT 'available',
    purchased_at TIMESTAMP NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_event_id ON tickets(event_id);