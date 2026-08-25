-- Idempotent development seed: clears event/ticket data and recreates a known
-- fixture. Re-running produces the same state rather than accumulating events.
--
-- Creates 50 tickets because that is the inventory the Phase 1 k6 contention
-- scenario expects (100 virtual users competing for 50 tickets).

BEGIN;

DELETE FROM ticket_cancellation_logs;
DELETE FROM tickets;
DELETE FROM events;

-- bcrypt digest of 'password123'. Generated once and pinned here rather than
-- computed at seed time, because bcrypt cannot be produced in SQL.
INSERT INTO users (email, password_hash, full_name, is_admin)
VALUES ('admin@example.com',
        '$2a$10$VthJ97JNlG4sLwfYb5i.7OVyXZrY1pigGVfXY4ZMuM85Sc61nV6pe',
        'Admin User', true)
ON CONFLICT (email) DO UPDATE
    SET is_admin      = true,
        full_name     = EXCLUDED.full_name,
        password_hash = EXCLUDED.password_hash;

WITH organizer AS (
    SELECT id FROM users WHERE email = 'admin@example.com'
), new_event AS (
    INSERT INTO events (name, description, location, start_time, end_time, organizer_id)
    SELECT 'Demo Concert',
           'Seeded event for local development',
           'San Francisco, CA',
           NOW() + INTERVAL '7 days',
           NOW() + INTERVAL '7 days 3 hours',
           organizer.id
    FROM organizer
    RETURNING id
)
INSERT INTO tickets (event_id, price, status)
SELECT new_event.id, 49.99, 'available'
FROM new_event, generate_series(1, 50);

COMMIT;

\echo 'Seeded: admin@example.com / password123, 1 event, 50 available tickets'
