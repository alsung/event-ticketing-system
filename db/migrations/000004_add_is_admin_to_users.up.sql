ALTER TABLE users ADD COLUMN is_admin BOOLEAN DEFAULT false NOT NULL;
UPDATE users SET is_admin = false WHERE is_admin IS NULL;