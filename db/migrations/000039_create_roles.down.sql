ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_fkey;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_system_role_fkey;

ALTER TABLE users
    ADD CONSTRAINT users_role_check CHECK (role IN ('artist', 'producer')),
    ADD CONSTRAINT users_system_role_check CHECK (system_role IN ('user', 'super_admin'));

DROP TABLE IF EXISTS roles;
