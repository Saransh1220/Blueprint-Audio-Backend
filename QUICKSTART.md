# Quick Start Guide - Waveyard Studio Backend

This guide walks you through setting up and developing on the **Waveyard Studio** backend locally.

---

## Prerequisites

- **Go**: `1.25+`
- **Docker & Docker Compose**
- **GNU Make** (or PowerShell equivalents)
- **golang-migrate CLI** tool

---

## Installation & Setup

### 1. Install golang-migrate CLI

```bash
# Windows (PowerShell) / Linux / macOS
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Verify installation
migrate -version
```

### 2. Configure Environment

```bash
# Copy template configuration
cp .env.example .env

# Windows (PowerShell)
Copy-Item .env.example .env

# Download Go dependencies
go mod download
```

---

## Standard Development Workflow

### Step 1: Start Infrastructure Containers
```bash
make docker-up
```
*Starts PostgreSQL (`5432`), Redis (`6379`), Prometheus (`9090`), and Grafana (`3000`).*

### Step 2: Apply Database Migrations
```bash
make migrate-up
```
*Applies all pending `.sql` migrations in `db/migrations/`.*

### Step 3: Run the API Server
```bash
make run
```
*Starts the HTTP API server on `http://localhost:8080`.*

### Step 4: Run the Media Worker (Separate Terminal)
```bash
make run-worker
```
*Starts the durable background worker that processes audio uploads and waveform peaks.*

### Step 5: Verify Health & Explore Swagger Docs
- **Health Check**: `curl http://localhost:8080/health`
- **Swagger UI**: `http://localhost:8080/docs/`
- **OpenAPI Schema**: `http://localhost:8080/openapi.yaml`

---

## Makefile Commands Reference

The project uses `make` commands for all common developer tasks. Run `make help` to see all available commands.

### Development & Execution
```bash
make run              # Run the HTTP API server locally
make run-worker       # Run the durable media processing worker locally
make build            # Compile binaries into bin/
make clean            # Remove build and coverage artifacts
make fmt              # Format Go source files
make lint             # Run golangci-lint
```

### Docker Infrastructure
```bash
make docker-up        # Start PostgreSQL, Redis, Prometheus, and Grafana in background
make docker-down      # Stop all Docker containers
make docker-build     # Build Docker image
make logs             # View logs from all containers
make logs-api         # View logs specifically from API container
make dev              # Convenience alias for docker-up
```

### Database Migrations
```bash
make migrate-create name=my_migration   # Create a new migration pair in db/migrations/
make migrate-up                         # Apply all pending migrations
make migrate-down                       # Roll back the most recent migration
make migrate-version                    # Show current schema version and dirty status
make migrate-force version=N            # Force database version (fix dirty states)
make migrate-drop                       # Drop all tables (dev only)
```

### Testing & Coverage
```bash
make test             # Run all tests
make test-unit        # Run unit tests
make test-integration # Run integration tests
make coverage         # Full coverage report dashboard
make coverage-report  # Non-blocking coverage report
make coverage-check COVERAGE_THRESHOLD=80 # CI coverage gate
```

---

## Migration System Explained

### How It Works
1. **Version Tracking**: The `schema_migrations` table tracks which migrations have run.
2. **Idempotent**: Already-run migrations are skipped.
3. **Sequential**: Migrations run in numerical order (000001, 000002, ...).
4. **Auto-Migrate in Production**: When `AUTO_MIGRATE=true`, pending migrations are applied automatically on startup before the server binds to its port.

### Troubleshooting Dirty Migration States
If a migration fails midway, `golang-migrate` marks the database as `dirty = true`:
```bash
# 1. Check version
make migrate-version
# Output: 36 (dirty: true)

# 2. Fix the SQL error in your migration file

# 3. Force database to last known good version
make migrate-force version=35

# 4. Re-run migrations
make migrate-up
```

---

## Local Service Ports

When running Docker infrastructure:

| Service | Address | Default Credentials |
| :--- | :--- | :--- |
| **API Server** | `http://localhost:8080` | - |
| **Swagger UI** | `http://localhost:8080/docs/` | - |
| **PostgreSQL** | `localhost:5432` | `postgres` / `postgres` (`waveyard`) |
| **Redis** | `localhost:6379` | - |
| **Prometheus** | `http://localhost:9090` | - |
| **Grafana** | `http://localhost:3000` | `admin` / `admin` |

