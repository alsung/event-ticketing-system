ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_message_id_key;
ALTER TABLE notifications DROP COLUMN IF EXISTS message_id;
ALTER TABLE notifications ADD CONSTRAINT notifications_ticket_id_event_type_key UNIQUE (ticket_id, event_type);
