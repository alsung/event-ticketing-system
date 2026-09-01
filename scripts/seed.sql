-- Idempotent development seed: clears event/ticket data and recreates a known
-- fixture. Re-running produces the same state rather than accumulating events.
--
-- Creates 50 tickets because that is the inventory the Phase 1 k6 contention
-- scenario expects (100 virtual users competing for 50 tickets).

BEGIN;

-- Order matters: payments and cancellation logs both reference tickets, and
-- tickets reference events. Deleting a parent first trips the foreign key.
DELETE FROM notifications;
DELETE FROM outbox;
DELETE FROM idempotency_keys;
DELETE FROM payments;
DELETE FROM ticket_cancellation_logs;
DELETE FROM tickets;
DELETE FROM events;

-- bcrypt digest of 'password123'. Generated once and pinned here rather than
-- computed at seed time, because bcrypt cannot be produced in SQL.
-- Two accounts, both with the digest of 'password123'. The organiser exists so
-- the organiser flow can be demonstrated without handing out admin rights.
INSERT INTO users (email, password_hash, full_name, role)
VALUES ('admin@example.com',
        '$2a$10$VthJ97JNlG4sLwfYb5i.7OVyXZrY1pigGVfXY4ZMuM85Sc61nV6pe',
        'Admin User', 'admin'),
       ('organizer@example.com',
        '$2a$10$VthJ97JNlG4sLwfYb5i.7OVyXZrY1pigGVfXY4ZMuM85Sc61nV6pe',
        'Ines Oyelaran', 'organizer')
ON CONFLICT (email) DO UPDATE
    SET role          = EXCLUDED.role,
        full_name     = EXCLUDED.full_name,
        password_hash = EXCLUDED.password_hash;

-- A dozen events with varied names, artists, venues and cities, so search has
-- something to actually discriminate between. One seeded event demonstrates
-- nothing about ranking.
WITH organizer AS (
    SELECT id FROM users WHERE email = 'admin@example.com'
), organiser2 AS (
    SELECT id FROM users WHERE email = 'organizer@example.com'
), seeded AS (
    INSERT INTO events (name, description, location, start_time, end_time, organizer_id)
    -- Alternate ownership so both seeded accounts have events to manage.
    SELECT v.name, v.description, v.location,
           NOW() + (v.days || ' days')::interval,
           NOW() + (v.days || ' days')::interval + interval '3 hours',
           CASE WHEN v.days % 2 = 0 THEN organiser2.id ELSE organizer.id END
    FROM organizer, organiser2, (VALUES
        ('Aurora Vale · Midnight Cartography',
         'Aurora Vale brings the Midnight Cartography tour to the Fillmore, with support from Low Ceiling.',
         'San Francisco, CA', 7),
        ('The Paper Kites of Winter',
         'An acoustic evening of folk from The Paper Kites of Winter, strings and harmonies only.',
         'Portland, OR', 9),
        ('Basalt Drift — Warehouse Session',
         'Techno producer Basalt Drift plays a four-hour warehouse set. Late doors, no support.',
         'Detroit, MI', 11),
        ('Marigold Sound Symphony',
         'The Marigold Sound Symphony performs Ravel and Debussy under conductor Ines Oyelaran.',
         'Chicago, IL', 12),
        ('Neon Harbour Festival · Day One',
         'Day one of Neon Harbour with Aurora Vale, Basalt Drift and eleven more across three stages.',
         'Seattle, WA', 14),
        ('Quiet Machines Live',
         'Post-rock four-piece Quiet Machines play the whole of Glasshouse front to back.',
         'Austin, TX', 16),
        ('Comedy Cellar Late Show',
         'Stand-up from Priya Raman, Marcus Bell and a surprise headliner. Two drink minimum.',
         'New York, NY', 18),
        ('Saltwater Choir · Cathedral Nights',
         'Saltwater Choir perform by candlelight in the cathedral nave. Limited seating.',
         'Boston, MA', 21),
        ('Ember & Ash — Farewell Tour',
         'The final show of the Ember and Ash farewell tour after fourteen years.',
         'Nashville, TN', 24),
        ('Kestrel Jazz Quartet',
         'Hard bop from the Kestrel Jazz Quartet, two sets with an interval.',
         'New Orleans, LA', 27),
        ('Static Bloom Album Launch',
         'Static Bloom launch Fault Lines with a full live band and a listening session.',
         'Los Angeles, CA', 30),
        ('Harbourlight Electronic Weekender',
         'Ambient and downtempo across a weekend, curated by Basalt Drift.',
         'Miami, FL', 33)
    ) AS v(name, description, location, days)
    RETURNING id
)
-- 50 tickets on the first event, which the k6 contention scenario expects;
-- a smaller spread on the rest so the listing looks like a real catalogue.
INSERT INTO tickets (event_id, price, status)
SELECT seeded.id,
       (30 + (row_number() OVER (ORDER BY seeded.id)) * 7)::numeric(10,2),
       'available'
FROM seeded,
     generate_series(1, 12);

-- Top the first event up to exactly 50 available tickets.
WITH first_event AS (
    SELECT id FROM events ORDER BY start_time LIMIT 1
)
INSERT INTO tickets (event_id, price, status)
SELECT first_event.id, 49.99, 'available'
FROM first_event, generate_series(1, 38);

COMMIT;

\echo 'Seeded: admin@example.com and organizer@example.com / password123, 12 events'
