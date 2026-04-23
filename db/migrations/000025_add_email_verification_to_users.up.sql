ALTER TABLE users
    ADD COLUMN IF NOT EXISTS email_verified BOOLEAN,
    ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMP WITH TIME ZONE;

UPDATE users
SET email_verified = true,
    email_verified_at = COALESCE(email_verified_at, created_at)
WHERE email_verified IS NULL;

ALTER TABLE users
    ALTER COLUMN email_verified SET DEFAULT false;

UPDATE users
SET email_verified = false
WHERE email_verified IS NULL;

ALTER TABLE users
    ALTER COLUMN email_verified SET NOT NULL;
