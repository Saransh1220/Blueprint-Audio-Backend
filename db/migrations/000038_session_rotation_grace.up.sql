-- Refresh-token rotation: remember when a session was rotated and which session replaced it,
-- so a concurrent refresh from another tab can be told apart from a logout/revoke.
ALTER TABLE user_sessions
    ADD COLUMN IF NOT EXISTS rotated_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS replaced_by UUID NULL;
