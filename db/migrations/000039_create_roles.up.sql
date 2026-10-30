-- Database-driven RBAC.
--
-- Roles and their permissions live in this table instead of Go code. To grant a
-- permission or add a role, write a new migration that updates `roles`; no
-- deploy-time code change is needed beyond the Permission constant the route
-- uses (middleware/permissions.go).
--
-- A user has two role axes, both referencing this table by name:
--   users.role         -> a role of kind 'business' (artist, producer)
--   users.system_role  -> a role of kind 'system'   (user, super_admin)
-- Effective permissions are the union of both roles' permissions.
CREATE TABLE IF NOT EXISTS roles (
    id          SMALLINT PRIMARY KEY,
    name        VARCHAR(50) NOT NULL UNIQUE,
    kind        VARCHAR(10) NOT NULL CHECK (kind IN ('business', 'system')),
    description TEXT NOT NULL DEFAULT '',
    permissions TEXT[] NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO roles (id, name, kind, description, permissions) VALUES
    (1, 'artist',      'business', 'Buys beats and samples',
        '{}'),
    (2, 'producer',    'business', 'Uploads and sells beats and samples',
        '{spec:upload,analytics:view,orders:producer}'),
    (3, 'user',        'system',   'Regular account with no administrative access',
        '{}'),
    (4, 'super_admin', 'system',   'Full administrative access',
        '{admin:users:read,admin:users:write,admin:roles:read,admin:roles:assign,admin:specs:read,admin:specs:write,admin:orders:read,admin:licenses:read,admin:analytics:read,admin:audit:read}')
ON CONFLICT (id) DO NOTHING;

-- Replace the hard-coded CHECK constraints with foreign keys so that new roles
-- can be introduced by inserting a row. The migration fails loudly if any
-- existing user holds a role that is not in the table above.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_system_role_check;

ALTER TABLE users
    ADD CONSTRAINT users_role_fkey
        FOREIGN KEY (role) REFERENCES roles (name) ON UPDATE CASCADE,
    ADD CONSTRAINT users_system_role_fkey
        FOREIGN KEY (system_role) REFERENCES roles (name) ON UPDATE CASCADE;
