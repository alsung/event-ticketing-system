-- The availability query and the purchase claim both filter on
-- (event_id, status). The existing idx_event_id covers only the first column, so
-- Postgres had to filter the status predicate after the index scan -- reading
-- every ticket for an event to find the available ones.
--
-- Column order matters: event_id first because it is always an equality match,
-- status second. A (status, event_id) index would be far less selective, since
-- status has two live values across the whole table.
CREATE INDEX IF NOT EXISTS idx_tickets_event_id_status
    ON tickets (event_id, status);

-- Superseded by the composite index above, which can serve any query the
-- single-column index could.
DROP INDEX IF EXISTS idx_event_id;
