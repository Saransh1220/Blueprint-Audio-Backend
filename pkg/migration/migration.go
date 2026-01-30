package migration

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

// Config holds migration configuration
type Config struct {
	MigrationsPath string
	DatabaseURL    string
	Logger         *slog.Logger
}

// Runner handles database migrations
type Runner struct {
	config *Config
	logger *slog.Logger
}

// NewRunner creates a new migration runner
func NewRunner(config *Config) *Runner {
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}

	return &Runner{
		config: config,
		logger: logger,
	}
}

// Up runs all pending migrations
func (r *Runner) Up() error {
	r.logger.Info("Running database migrations...")

	m, err := r.getMigrate()
	if err != nil {
		return fmt.Errorf("failed to initialize migrate: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil {
		if err == migrate.ErrNoChange {
			r.logger.Info("No new migrations to run")
			return nil
		}
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	r.logger.Info("Migrations completed successfully")
	return nil
}

// Down rolls back the last migration
func (r *Runner) Down() error {
	r.logger.Info("Rolling back last migration...")

	m, err := r.getMigrate()
	if err != nil {
		return fmt.Errorf("failed to initialize migrate: %w", err)
	}
	defer m.Close()

	if err := m.Steps(-1); err != nil {
		if err == migrate.ErrNoChange {
			r.logger.Info("No migrations to roll back")
			return nil
		}
		return fmt.Errorf("failed to rollback migration: %w", err)
	}

	r.logger.Info("Migration rolled back successfully")
	return nil
}

// Force sets the migration version without running migrations
// Use this carefully to fix broken migration states
func (r *Runner) Force(version int) error {
	r.logger.Warn("Forcing migration version", "version", version)

	m, err := r.getMigrate()
	if err != nil {
		return fmt.Errorf("failed to initialize migrate: %w", err)
	}
	defer m.Close()

	if err := m.Force(version); err != nil {
		return fmt.Errorf("failed to force version: %w", err)
	}

	r.logger.Info("Migration version forced successfully", "version", version)
	return nil
}

// Version returns the current migration version
func (r *Runner) Version() (uint, bool, error) {
	m, err := r.getMigrate()
	if err != nil {
		return 0, false, fmt.Errorf("failed to initialize migrate: %w", err)
	}
	defer m.Close()

	version, dirty, err := m.Version()
	if err != nil {
		if err == migrate.ErrNilVersion {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("failed to get version: %w", err)
	}

	return version, dirty, nil
}

// getMigrate creates a new migrate instance
func (r *Runner) getMigrate() (*migrate.Migrate, error) {
	// Open database connection
	db, err := sql.Open("postgres", r.config.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Create postgres driver instance
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres driver: %w", err)
	}

	// Create migrate instance
	m, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", r.config.MigrationsPath),
		"postgres",
		driver,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrate instance: %w", err)
	}

	return m, nil
}

// AutoMigrate runs migrations automatically on application start
// This is called from main.go on startup
func AutoMigrate(dbURL, migrationsPath string, logger *slog.Logger) error {
	runner := NewRunner(&Config{
		MigrationsPath: migrationsPath,
		DatabaseURL:    dbURL,
		Logger:         logger,
	})

	// Check current version
	version, dirty, err := runner.Version()
	if err != nil {
		logger.Error("Failed to get migration version", "error", err)
		return err
	}

	if dirty {
		logger.Warn("Database is in dirty state", "version", version)
		logger.Info("Please fix the migration manually or use 'make migrate-force' command")
		return fmt.Errorf("database in dirty state at version %d", version)
	}

	logger.Info("Current migration version", "version", version)

	// Run migrations
	if err := runner.Up(); err != nil {
		return err
	}

	// Get new version
	newVersion, _, err := runner.Version()
	if err != nil {
		return err
	}

	logger.Info("Migration completed", "from_version", version, "to_version", newVersion)
	return nil
}
