DROP INDEX IF EXISTS idx_events_search_vector;
ALTER TABLE events DROP COLUMN IF EXISTS search_vector;
