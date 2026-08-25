-- Passwords were stored in plaintext. Rename the column so the schema states
-- what it now holds: a bcrypt digest, not a password.
--
-- Existing values cannot be migrated. bcrypt cannot be computed in SQL, and
-- hashing them at the application layer would require reading every plaintext
-- password -- exactly what this change exists to stop. Dev rows are therefore
-- invalidated rather than converted; `make seed` recreates the admin account.
ALTER TABLE users RENAME COLUMN password TO password_hash;

-- A bcrypt digest is always 60 characters. Anything shorter is a leftover
-- plaintext value, which must not be allowed to authenticate.
UPDATE users SET password_hash = '' WHERE length(password_hash) <> 60;
