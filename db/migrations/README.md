# Database Migrations

This directory contains database migration files managed by [golang-migrate](https://github.com/golang-migrate/migrate).

## Migration Files

Migration files are created in pairs:
- `NNNNNN_name.up.sql` - Applies the migration
- `NNNNNN_name.down.sql` - Reverts the migration

Where `NNNNNN` is a sequential number (e.g., 000001, 000002).

## Creating Migrations

Use the Makefile command:

```bash
make migrate-create name=create_users_table
```

This will create two files:
- `db/migrations/NNNNNN_create_users_table.up.sql`
- `db/migrations/NNNNNN_create_users_table.down.sql`

## Available Commands

See the main project README or run `make help` for all migration commands.

## Important Notes

- Migrations are run automatically on application startup
- Already executed migrations are tracked and won't run again
- The `schema_migrations` table tracks which migrations have been applied
- Never modify migration files that have been applied to production
- Always test migrations with both `up` and `down` before deploying
