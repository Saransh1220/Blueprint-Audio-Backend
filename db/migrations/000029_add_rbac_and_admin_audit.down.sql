DROP TABLE IF EXISTS admin_audit_logs;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_system_role_check,
    DROP CONSTRAINT IF EXISTS users_status_check,
    DROP COLUMN IF EXISTS system_role,
    DROP COLUMN IF EXISTS status;
