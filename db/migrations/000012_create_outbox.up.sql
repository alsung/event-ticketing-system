-- Transactional outbox.
--
-- Postgres and Kafka share no transaction, so there is no way to commit a ticket
-- change and produce an event atomically. Committing first loses the event if
-- the process dies before producing; producing first announces a purchase that
-- may then roll back. Neither is acceptable for money-adjacent events.
--
-- Instead the event is written here inside the same transaction as the ticket
-- change, and a relay publishes it afterwards. The state change and the
-- intent-to-publish are then atomic.
--
-- The cost is at-least-once delivery: the relay can produce and then die before
-- marking the row sent, republishing on restart. Consumers are therefore
-- idempotent. Exactly-once across a database and a broker is not achievable
-- without distributed transactions.

CREATE TABLE IF NOT EXISTS outbox (
    -- BIGSERIAL, not UUID: the relay polls in insertion order, and a monotonic
    -- key makes that ordering free.
    id            BIGSERIAL PRIMARY KEY,

    -- The Kafka message key. Keying by ticket puts every event for one ticket on
    -- the same partition, so a cancellation can never be consumed before the
    -- purchase it refers to.
    aggregate_id  UUID NOT NULL,
    topic         TEXT NOT NULL,
    payload       JSONB NOT NULL,

    created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    published_at  TIMESTAMP,

    -- Retry bookkeeping, so a permanently failing event is visible rather than
    -- silently retried forever.
    attempts      INT NOT NULL DEFAULT 0,
    last_error    TEXT
);

-- Partial index: the relay only ever asks for unpublished rows. Indexing just
-- those keeps the index small no matter how large the table grows, since
-- published rows drop out of it entirely.
CREATE INDEX IF NOT EXISTS idx_outbox_unpublished
    ON outbox (id) WHERE published_at IS NULL;
