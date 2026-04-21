ALTER TABLE users
    ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMP WITH TIME ZONE;

UPDATE users
SET email_verified = true,
    email_verified_at = COALESCE(email_verified_at, created_at)
WHERE email_verified = false;
