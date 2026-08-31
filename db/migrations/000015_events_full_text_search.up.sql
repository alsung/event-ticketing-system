-- Full-text search over events.
--
-- Postgres rather than a separate search cluster. At this scale a dedicated
-- engine would add a synchronisation problem -- keeping an external index in
-- step with this table is the same dual-write hazard the outbox exists to
-- solve -- without solving a problem that exists yet. See the README for the
-- threshold where that trade flips.

-- A generated column rather than a trigger: Postgres keeps it in step on every
-- insert and update, with no application code and nothing to forget.
--
-- Weights rank a match by where it was found. A query matching an event's name
-- should outrank one that merely mentions the word in its description.
ALTER TABLE events
    ADD COLUMN IF NOT EXISTS search_vector tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(name, '')),        'A') ||
        setweight(to_tsvector('english', coalesce(description, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(location, '')),    'C')
    ) STORED;

-- GIN, not GIN's alternative: a tsvector is a set of lexemes, and GIN is the
-- index built for containment queries over sets. A btree cannot answer them.
CREATE INDEX IF NOT EXISTS idx_events_search_vector
    ON events USING GIN (search_vector);
