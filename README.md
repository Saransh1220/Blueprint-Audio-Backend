# Blueprint Audio Backend

Backend API for the Blueprint Audio platform.

## Tech Stack
- Go `1.25.1`
- PostgreSQL
- Redis
- Cloudflare R2 (S3-compatible object storage)
- Razorpay (payments)
- Docker + Docker Compose

## Repository Structure
- `cmd/server` - application entrypoint
- `internal/` - handlers, services, repositories, middleware, DTOs
- `db/migrations` - SQL migrations
- `pkg/migration` - migration runner utilities
- `tools/coverage-runner` - cross-platform test+coverage artifact generator
- `tools/coverage-report` - coverage/failure report generator and dashboard

## Prerequisites
- Go `1.25+`
- Docker + Docker Compose
- GNU Make
- `migrate` CLI (for manual migration commands)
- Cloudflare R2 bucket + API credentials (free tier is fine for development)

Install migrate CLI (example):
```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

## Quick Start
1. Copy environment file:
```bash
cp .env.example .env
```
(Windows PowerShell)
```powershell
Copy-Item .env.example .env
```

2. Fill `.env` with your Cloudflare R2 + DB + Razorpay values.

3. Start dependencies:
```bash
make docker-up
```

4. Run migrations:
```bash
make migrate-up
```

5. Run API locally:
```bash
make run
```

Health check:
```bash
curl http://localhost:8080/health
```

## Environment Variables
Use `.env.example` as template.

### Server
- `PORT` (default `8080`)
- `ENV`
- `ALLOWED_ORIGINS` (comma-separated)

### Database
- `DB_HOST`
- `DB_PORT`
- `DB_USER`
- `DB_PASSWORD`
- `DB_NAME`

### Auth
- `JWT_SECRET`
- `JWT_EXPIRATION` (example `24h`)

### Storage (Cloudflare R2)
Required:
- `S3_ENDPOINT` (example `https://<account_id>.r2.cloudflarestorage.com`)
- `S3_ACCESS_KEY`
- `S3_SECRET_KEY`
- `S3_BUCKET`

Recommended:
- `S3_REGION=auto`
- `S3_USE_SSL=true`
- `S3_PUBLIC_ENDPOINT=` (optional)

### Payments
- `RAZORPAY_KEY_ID`
- `RAZORPAY_KEY_SECRET`

### Redis
- `REDIS_HOST`
- `REDIS_PORT`
- `REDIS_PASSWORD`

## Common Commands
```bash
make help             # list commands
make build            # build binary -> bin/blueprint-audio
make run              # run API
make test             # tidy + run all tests
make clean            # cleanup artifacts
```

## Testing and Coverage
### Tests
```bash
make test
make test-unit
make test-integration
```

### Coverage (Professional Workflow)
```bash
make coverage
```
- Strict mode (fails if tests fail)
- Generates:
  - `coverage/coverage.html` (dashboard home)
  - `coverage/coverage-details.html` (line-by-line coverage)
  - `coverage/summary.md`
  - `coverage/summary.json`
  - `coverage/functions.txt`
  - `coverage/test-report.jsonl`

```bash
make coverage-report
```
- Non-blocking mode (generates report even if tests fail)

```bash
make coverage-check COVERAGE_THRESHOLD=70
```
- CI gate for coverage threshold + test success

## Database Migrations
```bash
make migrate-create name=add_new_table
make migrate-up
make migrate-down
make migrate-version
make migrate-force version=3
make migrate-drop
```

## Docker
```bash
make docker-build
make docker-up
make docker-down
make logs
```

Default local services from `docker-compose.yml`:
- API: `http://localhost:8080`
- PostgreSQL: `${DB_PORT}` -> container `5432`
- Redis: `6379`

## Key API Routes
- `GET /health`
- `POST /register`
- `POST /login`
- `GET /me`
- `GET /specs`
- `GET /specs/{id}`
- `POST /specs`
- `PATCH /specs/{id}`
- `DELETE /specs/{id}`
- `PATCH /users/profile`
- `POST /users/profile/avatar`
- `GET /users/{id}/public`
- `GET /users/{id}/specs`
- `POST /orders`
- `GET /orders`
- `GET /orders/{id}`
- `POST /payments/verify`
- `GET /licenses`
- `GET /licenses/{id}/downloads`
- `POST /specs/{id}/play`
- `POST /specs/{id}/download-free`
- `POST /specs/{id}/favorite`
- `GET /specs/{id}/analytics`
- `GET /analytics/overview`

## CI Notes
- CI runs tests and coverage check (`70%` threshold by default).
- Commit both `go.mod` and `go.sum` to avoid checksum-related CI failures.
