.PHONY: help build run test clean migrate-up migrate-down migrate-create docker-build docker-up docker-down dev

# Variables
APP_NAME=redwave-api
DOCKER_COMPOSE=docker-compose
GO=go
MIGRATE=migrate

# Help command
help:
	@echo "Available commands:"
	@echo "  make build          - Build the Go binary"
	@echo "  make run            - Run the application locally"
	@echo "  make test           - Run tests"
	@echo "  make test-coverage  - Run tests with coverage"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make migrate-up     - Run database migrations up"
	@echo "  make migrate-down   - Rollback last migration"
	@echo "  make migrate-create - Create new migration (name=migration_name)"
	@echo "  make migrate-force  - Force migration version (version=N)"
	@echo "  make migrate-version- Show current migration version"
	@echo "  make migrate-drop   - Drop all migrations (DANGEROUS)"
	@echo "  make docker-build   - Build Docker images"
	@echo "  make docker-up      - Start all Docker containers"
	@echo "  make docker-down    - Stop all Docker containers"
	@echo "  make dev            - Start development environment"
	@echo "  make logs           - View Docker logs"

# Build the application
build:
	@echo "Building $(APP_NAME)..."
	$(GO) build -o bin/$(APP_NAME) ./cmd/server

# Run the application
run:
	@echo "Running $(APP_NAME)..."
	$(GO) run ./cmd/server/main.go

# Run tests
test:
	@echo "Running tests..."
	$(GO) test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html

# Database migrations
DB_URL=postgresql://postgres:postgres@localhost:5432/redwave?sslmode=disable

migrate-up:
	@echo "Running migrations up..."
	$(MIGRATE) -path db/migrations -database "$(DB_URL)" up

migrate-down:
	@echo "Rolling back last migration..."
	$(MIGRATE) -path db/migrations -database "$(DB_URL)" down 1

migrate-create:
	@powershell -Command "if ('$(name)' -eq '') { Write-Host 'Error: Please provide migration name using name=your_migration_name'; Write-Host 'Example: make migrate-create name=create_products_table'; exit 1 }"
	@echo "Creating migration: $(name)"
	$(MIGRATE) create -ext sql -dir db/migrations -seq $(name)
	@echo "Migration files created successfully!"
	@echo "Edit the files in db/migrations/"

migrate-force:
	@powershell -Command "if ('$(version)' -eq '') { Write-Host 'Error: Please provide version using version=N'; Write-Host 'Example: make migrate-force version=1'; exit 1 }"
	@echo "Forcing migration version to $(version)..."
	@echo "WARNING: This will mark the database as being at version $(version) without running migrations!"
	@powershell -Command "$$response = Read-Host 'Are you sure? [y/N]'; if ($$response -eq 'y' -or $$response -eq 'Y') { exit 0 } else { Write-Host 'Operation cancelled'; exit 1 }"
	$(MIGRATE) -path db/migrations -database "$(DB_URL)" force $(version)
	@echo "Migration version forced to $(version)"

migrate-version:
	@echo "Current migration version:"
	@$(MIGRATE) -path db/migrations -database "$(DB_URL)" version

migrate-drop:
	@echo "WARNING: This will drop all tables and remove all data!"
	@powershell -Command "$$response = Read-Host 'Are you sure? [y/N]'; if ($$response -eq 'y' -or $$response -eq 'Y') { exit 0 } else { Write-Host 'Operation cancelled'; exit 1 }"
	$(MIGRATE) -path db/migrations -database "$(DB_URL)" drop
	@echo "All migrations dropped"

# Docker commands
docker-build:
	@echo "Building Docker images..."
	$(DOCKER_COMPOSE) build

docker-up:
	@echo "Starting Docker containers..."
	$(DOCKER_COMPOSE) up -d
	@echo "Waiting for services to be ready..."
	@sleep 5
	@echo "Services started. Access:"
	@echo "  API: http://localhost:8080"
	@echo "  MinIO Console: http://localhost:9001 (user: minioadmin, pass: minioadmin)"
	@echo "  PostgreSQL: localhost:5432"

docker-down:
	@echo "Stopping Docker containers..."
	$(DOCKER_COMPOSE) down

docker-logs:
	@echo "Showing Docker logs..."
	$(DOCKER_COMPOSE) logs -f

logs: docker-logs

# Development environment
dev: docker-up
	@echo "Development environment ready!"
	@echo "Run 'make logs' to view container logs"

# Format code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

# Lint code (requires golangci-lint)
lint:
	@echo "Linting code..."
	golangci-lint run ./...

# Initialize MinIO bucket for local development
minio-setup:
	@echo "Setting up MinIO bucket..."
	docker exec redwave_minio mc alias set local http://localhost:9000 minioadmin minioadmin
	docker exec redwave_minio mc mb local/redwave-assets || true
	docker exec redwave_minio mc policy set public local/redwave-assets
	@echo "MinIO bucket 'redwave-assets' created and set to public"
