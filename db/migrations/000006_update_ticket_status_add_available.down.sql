ALTER TABLE tickets DROP CONSTRAINT tickets_status_check;
ALTER TABLE tickets
ADD CONSTRAINT tickets_status_check
CHECK (status IN ('reserved', 'purchased', 'cancelled'));

ALTER TABLE tickets ALTER COLUMN status SET DEFAULT 'reserved';

UPDATE tickets SET status = 'reserved' WHERE status = 'available';