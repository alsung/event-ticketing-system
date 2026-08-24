ALTER TABLE tickets DROP CONSTRAINT tickets_status_check;
ALTER TABLE tickets
ADD CONSTRAINT tickets_status_check
CHECK (status IN ('available', 'reserved', 'purchased', 'cancelled'));

ALTER TABLE tickets ALTER COLUMN status SET DEFAULT 'available';

UPDATE tickets SET status = 'available' WHERE status = 'reserved';