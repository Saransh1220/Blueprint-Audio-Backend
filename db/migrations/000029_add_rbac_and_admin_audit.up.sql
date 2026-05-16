ALTER TABLE users
    ADD COLUMN IF NOT EXISTS system_role VARCHAR(50) NOT NULL DEFAULT 'user',
    ADD COLUMN IF NOT EXISTS status VARCHAR(50) NOT NULL DEFAULT 'active';

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_system_role_check,
    ADD CONSTRAINT users_system_role_check
        CHECK (system_role IN ('user', 'super_admin'));

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_status_check,
    ADD CONSTRAINT users_status_check
        CHECK (status IN ('active', 'suspended'));

CREATE TABLE IF NOT EXISTS admin_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID,
    before_state JSONB,
    after_state JSONB,
    ip_address VARCHAR(100),
    user_agent TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_system_role ON users(system_role);
CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_actor_id ON admin_audit_logs(actor_id);
CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_resource ON admin_audit_logs(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_created_at ON admin_audit_logs(created_at DESC);
