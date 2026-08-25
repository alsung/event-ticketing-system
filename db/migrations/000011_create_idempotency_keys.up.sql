-- Application-level request deduplication.
--
-- This is the first of two idempotency layers. It catches the common case: a
-- double-clicked button, a mobile client retrying on a flaky connection, a load
-- balancer replaying a request. The second layer is Stripe's own idempotency
-- key, which covers the window this table cannot -- between calling Stripe and
-- committing locally.

CREATE TABLE IF NOT EXISTS idempotency_keys (
    key             TEXT PRIMARY KEY,

    -- Keys are scoped per caller so one user cannot observe or collide with
    -- another user's key.
    user_id         UUID NOT NULL REFERENCES users(id),
    endpoint        TEXT NOT NULL,

    -- SHA-256 of the request body. A repeated key with a different body is a
    -- client bug worth surfacing loudly (422) rather than silently guessing
    -- which request was meant.
    request_hash    TEXT NOT NULL,

    -- NULL while the operation is still in flight. A second request arriving
    -- during that window gets 409 and should retry.
    response_status INT,
    response_body   JSONB,

    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMP
);

-- Keys are reaped after 24h; this index backs the sweep.
CREATE INDEX IF NOT EXISTS idx_idempotency_keys_created_at ON idempotency_keys (created_at);
