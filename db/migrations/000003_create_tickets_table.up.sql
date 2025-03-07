CREATE TABLE tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID REFERENCES events(id),
    user_id UUID REFERENCES users(id),
    qr_code TEXT UNIQUE,
    status TEXT CHECK(status IN ('reserved', 'purchased', 'cancelled')) NOT NULL DEFAULT 'reserved',
    purchased_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW()
);