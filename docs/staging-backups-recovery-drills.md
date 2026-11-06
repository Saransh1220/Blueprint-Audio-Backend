# Staging, Backups and Recovery Drills

This document establishes the operational procedures for environment staging, database backups, restore verification, and migration recovery for Waveyard (`blueprint-audio`).

---

## 1. Staging Environment & Feature Flagging

### Environment Configurations
* **Development**: Local environment with Dockerized PostgreSQL and MinIO. `ENV=development`, `TRUST_PROXY_HEADERS=false`.
* **Staging**: Pre-production target mirroring production topology. `ENV=staging`, strict HTTPS, isolated database, mock/sandbox payment gateways.
* **Production**: Live service with strict authorization, rate limiting, and authenticated monitoring. `ENV=production`.

### Feature Flagging & Safe Route Gating
* Unreleased or in-development sections are gated via the shared availability policy in [`red-wave-app/src/app/core/config/feature-availability.ts`](file:///C:/Users/saran/OneDrive/Desktop/projects/red-wave-app/src/app/core/config/feature-availability.ts).
* Routes configured in `UNRELEASED_ROUTES` (such as loops, sample packs, battles, earnings, and studio messages) automatically redirect to a friendly `/unavailable` placeholder until their backend modules and schemas are fully staged and validated.

---

## 2. Database Backup & Restore Drills

### Automated Backup Procedure
Use custom-format compressed PostgreSQL dumps (`-Fc`):
```bash
# Set credentials
export PGPASSWORD="$DB_PASSWORD"

# Create a consistent point-in-time backup
pg_dump -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" \
  -Fc --no-owner --no-acl --verbose \
  -f "backups/waveyard_$(date +%Y%m%d_%H%M%S).dump"
```

### Restore Verification Drill
Perform periodic restore drills into an isolated verification database to confirm backup integrity:
```bash
# 1. Create temporary drill database
createdb -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" waveyard_restore_drill

# 2. Restore from snapshot
pg_restore -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d waveyard_restore_drill \
  --clean --if-exists --no-owner --verbose "backups/waveyard_<timestamp>.dump"

# 3. Verify core data integrity
psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d waveyard_restore_drill -c "
  SELECT 'users' AS table_name, count(*) FROM users
  UNION ALL
  SELECT 'specs', count(*) FROM specs
  UNION ALL
  SELECT 'orders', count(*) FROM orders
  UNION ALL
  SELECT 'licenses', count(*) FROM licenses;
"

# 4. Clean up drill database
dropdb -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" waveyard_restore_drill
```

---

## 3. Database Migration Recovery Drills

Database migrations are managed using `golang-migrate` through the repository `Makefile`.

### Checking Migration State
```bash
make migrate-version
```

### Practicing Rollback
Before applying any migration to staging or production, test the rollback path locally:
```bash
# 1. Apply forward migrations
make migrate-up

# 2. Rollback the last migration
make migrate-down

# 3. Verify schema state and re-apply
make migrate-up
```

### Recovering from a Dirty Migration State
If a migration fails midway (e.g. network blip or lock timeout), the database is flagged as `dirty` and blocks further executions:
```bash
# 1. Inspect the last successful migration version from db/migrations
# 2. Force the migration version to the last known clean version N:
make migrate-force version=N

# 3. Fix the underlying SQL issue in db/migrations/<version>_*.sql
# 4. Re-run migration:
make migrate-up
```
