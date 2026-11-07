package fx

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dummyAuditLogger struct{}

func (dummyAuditLogger) Log(ctx context.Context, actorID *any, action, resourceType, resourceID string, beforeState, afterState any, ip, userAgent string) error {
	return nil
}

func TestModuleLifecycle(t *testing.T) {
	dbMock, _, err := sqlmock.New()
	require.NoError(t, err)
	defer dbMock.Close()

	sqlxDB := sqlx.NewDb(dbMock, "sqlmock")

	cfg := config.FXConfig{
		Enabled:           true,
		ProviderURL:       "https://api.frankfurter.app",
		APIKey:            "test-key",
		RefreshInterval:   24 * time.Hour,
		StaleAfter:        36 * time.Hour,
		MaxAge:            7 * 24 * time.Hour,
		MaxDailyMove:      0.20,
		ReportingCurrency: "INR",
	}

	m := NewModule(sqlxDB, cfg, nil)
	require.NotNil(t, m)
	assert.NotNil(t, m.Handler())
	assert.NotNil(t, m.Service())
	assert.NotNil(t, m.Snapshotter())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	m.StartRefresher(ctx)
}

func TestModuleDisabledRefresher(t *testing.T) {
	dbMock, _, err := sqlmock.New()
	require.NoError(t, err)
	defer dbMock.Close()

	sqlxDB := sqlx.NewDb(dbMock, "sqlmock")

	cfg := config.FXConfig{
		Enabled:           false,
		ProviderURL:       "https://api.frankfurter.app",
		RefreshInterval:   24 * time.Hour,
		StaleAfter:        36 * time.Hour,
		MaxAge:            7 * 24 * time.Hour,
		MaxDailyMove:      0.20,
		ReportingCurrency: "INR",
	}

	m := NewModule(sqlxDB, cfg, nil)
	require.NotNil(t, m)

	ctx := context.Background()
	m.StartRefresher(ctx) // enabled is false, does not spawn loop
}
