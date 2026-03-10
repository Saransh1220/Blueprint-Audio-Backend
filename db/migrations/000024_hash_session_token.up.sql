-- Rename refresh_token to refresh_token_digest and resize to 64 chars (SHA-256 hex)
ALTER TABLE user_sessions
    RENAME COLUMN refresh_token TO refresh_token_digest;

ALTER TABLE user_sessions
    ALTER COLUMN refresh_token_digest TYPE VARCHAR(64);
