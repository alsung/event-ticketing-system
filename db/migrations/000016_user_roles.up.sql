-- A role, replacing the single is_admin boolean.
--
-- A boolean can only express "staff or not". Organisers are a third thing: they
-- create and manage their own events but have no authority over anyone else's,
-- which is an ownership question rather than a privilege level. The role names
-- who you are; events.organizer_id decides what you may touch.

ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'attendee';

-- Backfill before constraining, so existing admins keep their access.
UPDATE users SET role = 'admin' WHERE is_admin = true;

ALTER TABLE users
    ADD CONSTRAINT users_role_check
    CHECK (role IN ('attendee', 'organizer', 'admin'));

-- The listing of an organiser's own events filters on this.
CREATE INDEX IF NOT EXISTS idx_users_role ON users (role) WHERE role <> 'attendee';

-- is_admin is now derivable from role, and two sources of truth for one fact is
-- how they drift apart.
ALTER TABLE users DROP COLUMN IF EXISTS is_admin;
