-- Revert NOT NULL constraint on display_name
ALTER TABLE users ALTER COLUMN display_name DROP NOT NULL;
