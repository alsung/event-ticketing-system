CREATE INDEX IF NOT EXISTS idx_event_id ON tickets (event_id);
DROP INDEX IF EXISTS idx_tickets_event_id_status;
