package migration_test

import (
	"log/slog"
	"testing"

	"github.com/saransh1220/blueprint-audio/pkg/migration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRunner_DefaultLogger(t *testing.T) {
	r := migration.NewRunner(&migration.Config{
		MigrationsPath: "db/migrations",
		DatabaseURL:    "postgres://invalid",
	})
	require.NotNil(t, r)
}

func TestRunnerMethods_InvalidConfig(t *testing.T) {
	r := migration.NewRunner(&migration.Config{
		MigrationsPath: "db/migrations",
		DatabaseURL:    "bad://url",
		Logger:         slog.Default(),
	})

	assert.Error(t, r.Up())
	assert.Error(t, r.Down())
	assert.Error(t, r.Force(1))
	_, _, err := r.Version()
	assert.Error(t, err)
}
