ALTER TABLE user_sessions
    DROP COLUMN IF EXISTS replaced_by,
    DROP COLUMN IF EXISTS rotated_at;
