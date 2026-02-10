-- Add NOT NULL constraint to display_name column
-- First, update existing users with NULL display_name to use their name
UPDATE users SET display_name = name WHERE display_name IS NULL OR display_name = '';

-- Now add the NOT NULL constraint
ALTER TABLE users ALTER COLUMN display_name SET NOT NULL;
