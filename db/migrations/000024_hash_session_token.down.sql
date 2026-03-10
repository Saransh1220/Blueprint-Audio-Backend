-- Revert: rename refresh_token_digest back to refresh_token and widen to 512 chars
ALTER TABLE user_sessions
    ALTER COLUMN refresh_token_digest TYPE VARCHAR(512);

ALTER TABLE user_sessions
    RENAME COLUMN refresh_token_digest TO refresh_token;
