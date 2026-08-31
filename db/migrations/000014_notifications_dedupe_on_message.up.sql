-- Deduplicate on the message, not on the ticket.
--
-- The original key was (ticket_id, event_type). That conflates two different
-- things: the same message delivered twice, which must be discarded, and the
-- same ticket genuinely purchased twice, which must not be. Cancelling returns
-- a seat to the pool, so a ticket really can be bought again -- and the second
-- buyer silently received no confirmation.
--
-- At-least-once delivery means the thing to deduplicate is the message. Each
-- outbox event carries a message id generated when it is enqueued.

ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_ticket_id_event_type_key;

ALTER TABLE notifications ADD COLUMN IF NOT EXISTS message_id UUID;

-- Existing rows predate message ids; give them synthetic ones so the column can
-- be made NOT NULL and unique.
UPDATE notifications SET message_id = gen_random_uuid() WHERE message_id IS NULL;

ALTER TABLE notifications ALTER COLUMN message_id SET NOT NULL;
ALTER TABLE notifications ADD CONSTRAINT notifications_message_id_key UNIQUE (message_id);
