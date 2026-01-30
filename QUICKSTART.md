# Quick Start Guide - Red Wave Backend

This guide shows you how to get started with the Red Wave backend development environment.

---

## Prerequisites

- Go 1.21+
- Docker & Docker Compose
- Make (for running commands)
- golang-migrate CLI tool

---

## Installation

### 1. Install golang-migrate

```bash
# Windows (PowerShell)
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Verify installation
migrate -version
```

### 2. Clone and Setup

```bash
cd c:\Users\saran\OneDrive\Desktop\projects\blueprint-backend

# Copy environment file
copy .env.example .env

# Install Go dependencies
go mod download
```

---

## Makefile Commands Reference

The project uses `make` commands for common tasks. Run `make help` to see all available commands.

### Development Commands

```bash
# Show all available commands
make help

# Start full development environment (Docker containers)
make dev
# This starts: PostgreSQL, Redis, MinIO, and the Go API

# Stop all containers
make docker-down

# View logs from all containers
make logs
```

### Build & Run Commands

```bash
# Build the Go binary
make build
# Output: bin/redwave-api

# Run the application locally (without Docker)
make run
# Starts server on http://localhost:8080
```

### Testing Commands

```bash
# Run all tests
make test

# Run tests with coverage report
make test-coverage
# Opens coverage.html in browser

# Clean build artifacts
make clean
```

### Migration Commands

#### Creating Migrations

```bash
# Create a new migration
make migrate-create name=create_users_table

# This creates TWO files:
# db/migrations/000001_create_users_table.up.sql
# db/migrations/000001_create_users_table.down.sql
```

**Example: Create users table**

1. Run the create command:
   ```bash
   make migrate-create name=create_users_table
   ```

2. Edit `000001_create_users_table.up.sql`:
   ```sql
   CREATE TABLE users (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       email VARCHAR(255) UNIQUE NOT NULL,
       name VARCHAR(100) NOT NULL,
       created_at TIMESTAMP DEFAULT NOW()
   );
   ```

3. Edit `000001_create_users_table.down.sql`:
   ```sql
   DROP TABLE IF EXISTS users;
   ```

#### Running Migrations

```bash
# Apply all pending migrations
make migrate-up

# Rollback the last migration
make migrate-down

# Check current migration version
make migrate-version
# Output: Current migration version: 5 (dirty: false)
```

#### Advanced Migration Commands

```bash
# Force migration to specific version (use carefully!)
make migrate-force version=3

# Drop ALL migrations (DANGEROUS - dev only)
make migrate-drop
```

---

## Common Workflows

### Workflow 1: Starting Development

```bash
# 1. Start all services
make dev

# 2. Wait for services to start (~5 seconds)

# 3. Setup MinIO bucket (first time only)
make minio-setup

# 4. Run migrations
make migrate-up

# 5. Start coding! API is at http://localhost:8080
```

### Workflow 2: Creating a New Migration

```bash
# 1. Create migration files
make migrate-create name=add_profile_picture_to_users

# 2. Edit the .up.sql file with your changes
# Example: ALTER TABLE users ADD COLUMN profile_pic VARCHAR(255);

# 3. Edit the .down.sql file with rollback
# Example: ALTER TABLE users DROP COLUMN profile_pic;

# 4. Apply the migration
make migrate-up

# 5. Test rollback (optional)
make migrate-down
make migrate-up
```

### Workflow 3: Running Tests

```bash
# Run tests
make test

# If tests fail, fix code and re-run
make test

# Generate coverage report
make test-coverage
```

### Workflow 4: Building for Production

```bash
# Build optimized binary
make build

# Run the binary
./bin/redwave-api

# Or build Docker image
make docker-build
```

---

## Service URLs

When running `make dev`:

| Service | URL | Credentials |
|---------|-----|-------------|
| **API** | http://localhost:8080 | - |
| **Database** | localhost:5432 | postgres/postgres |
| **Redis** | localhost:6379 | - |
| **MinIO Console** | http://localhost:9001 | minioadmin/minioadmin |
| **MinIO API** | localhost:9000 | minioadmin/minioadmin |

### Health Check

```bash
# Check if API is running
curl http://localhost:8080/health

# Response: {"status":"healthy"}
```

---

## Migration System Explained

### How It Works

1. **Automatic on Startup**: Migrations run automatically when the server starts
2. **Version Tracking**: `schema_migrations` table tracks which migrations have run
3. **Idempotent**: Already-run migrations are skipped (won't run twice)
4. **Sequential**: Migrations run in order (000001, 000002, 000003...)

### Migration File Naming

```
000001_create_users_table.up.sql   ← Sequential number + description
000001_create_users_table.down.sql
000002_add_indexes.up.sql
000002_add_indexes.down.sql
```

- **Numbers**: Auto-incremented (000001, 000002, etc.)
- **Name**: Descriptive (use underscores)
- **Extension**: `.up.sql` for apply, `.down.sql` for rollback

### What Happens When You Run `make migrate-up`?

```
1. Connect to database
2. Check current version (e.g., version 3)
3. Find all .up.sql files > version 3
4. Execute them in order: 000004, 000005, 000006...
5. Update schema_migrations table
6. Done!
```

### What Happens on Server Startup?

```go
// In cmd/server/main.go
migration.AutoMigrate(dbURL, migrationsPath, logger)
// This runs migrate-up automatically!
```

---

## Troubleshooting

### "Database in dirty state"

```bash
# Check version
make migrate-version
# Output: 5 (dirty: true)

# Fix by forcing to last known good version
make migrate-force version=4

# Then re-run
make migrate-up
```

### "Migration failed"

```bash
# 1. Check what failed
make migrate-version

# 2. Rollback
make migrate-down

# 3. Fix the SQL in the migration file

# 4. Try again
make migrate-up
```

### "Cannot connect to database"

```bash
# Make sure Docker containers are running
make docker-up

# Or restart everything
make docker-down
make dev
```

---

## Environment Variables

Edit `.env` to configure:

```env
# Database
DB_HOST=localhost
DB_PORT=5432
DB_NAME=redwave

# Server
PORT=8080

# S3/MinIO (local dev)
S3_ENDPOINT=localhost:9000
S3_BUCKET=redwave-assets

# Razorpay
RAZORPAY_KEY_ID=rzp_test_xxx
RAZORPAY_KEY_SECRET=xxx
```

---

## Next Steps

1. ✅ Review architecture docs in the `brain` folder
2. ✅ Create your first migration: `make migrate-create name=create_users_table`
3. ✅ Start development: `make dev`
4. ✅ Run migrations: `make migrate-up`
5. ✅ Build your features!

---

## Full Command Reference

### Development
- `make dev` - Start all services
- `make docker-up` - Start Docker containers
- `make docker-down` - Stop Docker containers
- `make logs` - View container logs
- `make minio-setup` - Initialize MinIO bucket

### Build & Run
- `make build` - Build Go binary
- `make run` - Run locally
- `make clean` - Remove build artifacts

### Testing
- `make test` - Run tests
- `make test-coverage` - Run with coverage
- `make fmt` - Format code
- `make lint` - Lint code

### Migrations
- `make migrate-create name=xxx` - Create new migration
- `make migrate-up` - Apply all pending
- `make migrate-down` - Rollback last
- `make migrate-version` - Show current version
- `make migrate-force version=N` - Force version
- `make migrate-drop` - Drop all (dev only)

### Docker
- `make docker-build` - Build images
- `make docker-up` - Start containers
- `make docker-down` - Stop containers

---

**Questions?** Check the detailed docs:
- [README.md](file:///C:/Users/saran/.gemini/antigravity/brain/6aaccb2b-740f-4c23-814c-bcd0569734be/README.md) - Overview
- [database_design.md](file:///C:/Users/saran/.gemini/antigravity/brain/6aaccb2b-740f-4c23-814c-bcd0569734be/database_design.md) - Migration details
- [system_architecture.md](file:///C:/Users/saran/.gemini/antigravity/brain/6aaccb2b-740f-4c23-814c-bcd0569734be/system_architecture.md) - Architecture
