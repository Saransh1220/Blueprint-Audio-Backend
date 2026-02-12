package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpecFinderMethods(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer sqlDB.Close()
	db := sqlx.NewDb(sqlDB, "sqlmock")
	repo := postgres.NewSpecRepository(db)
	ctx := context.Background()
	specID := uuid.New()
	licenseID := uuid.New()

	rows := sqlmock.NewRows([]string{"id", "producer_id", "producer_name", "title", "category", "type", "bpm", "key", "image_url", "preview_url", "wav_url", "stems_url", "base_price", "description", "duration", "free_mp3_enabled", "created_at", "updated_at", "is_deleted"}).
		AddRow(specID, uuid.New(), "p", "t", "beat", "wav", 120, "C", "img", "prev", nil, nil, 10.0, "d", 60, false, time.Now(), time.Now(), false)
	mock.ExpectQuery(`SELECT s\.\*, u\.display_name as producer_name`).WithArgs(specID).WillReturnRows(rows)
	mock.ExpectQuery(`SELECT \* FROM license_options WHERE spec_id = \$1 AND is_deleted = FALSE`).WithArgs(specID).WillReturnRows(sqlmock.NewRows([]string{"id", "spec_id", "license_type", "name", "price", "features", "file_types", "is_deleted"}))
	mock.ExpectQuery(`SELECT g\.\* FROM genres g JOIN spec_genres sg ON g\.id = sg\.genre_id WHERE sg\.spec_id = \$1`).WithArgs(specID).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "created_at"}))

	spec, err := repo.FindByID(ctx, specID)
	require.NoError(t, err)
	require.NotNil(t, spec)

	rows2 := sqlmock.NewRows([]string{"id", "producer_id", "producer_name", "title", "category", "type", "bpm", "key", "image_url", "preview_url", "wav_url", "stems_url", "base_price", "description", "duration", "free_mp3_enabled", "created_at", "updated_at", "is_deleted"}).
		AddRow(specID, uuid.New(), "p", "t", "beat", "wav", 120, "C", "img", "prev", nil, nil, 10.0, "d", 60, false, time.Now(), time.Now(), false)
	mock.ExpectQuery(`SELECT s\.\*, u\.display_name as producer_name`).WithArgs(specID).WillReturnRows(rows2)
	mock.ExpectQuery(`SELECT \* FROM license_options WHERE spec_id = \$1 AND is_deleted = FALSE`).WithArgs(specID).WillReturnRows(sqlmock.NewRows([]string{"id", "spec_id", "license_type", "name", "price", "features", "file_types", "is_deleted"}))
	mock.ExpectQuery(`SELECT g\.\* FROM genres g JOIN spec_genres sg ON g\.id = sg\.genre_id WHERE sg\.spec_id = \$1`).WithArgs(specID).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "created_at"}))
	_, err = repo.FindWithLicenses(ctx, specID)
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM specs WHERE id = \$1\)`).WithArgs(specID).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	exists, err := repo.Exists(ctx, specID)
	require.NoError(t, err)
	assert.True(t, exists)

	licRows := sqlmock.NewRows([]string{"id", "spec_id", "license_type", "name", "price", "features", "file_types", "is_deleted"}).AddRow(licenseID, specID, "Basic", "Basic", 9.99, "{}", "{}", false)
	mock.ExpectQuery(`SELECT \* FROM license_options WHERE id = \$1 AND is_deleted = FALSE`).WithArgs(licenseID).WillReturnRows(licRows)
	license, err := repo.GetLicenseByID(ctx, licenseID)
	require.NoError(t, err)
	require.NotNil(t, license)

	missing := uuid.New()
	mock.ExpectQuery(`SELECT \* FROM license_options WHERE id = \$1 AND is_deleted = FALSE`).WithArgs(missing).WillReturnError(sql.ErrNoRows)
	_, err = repo.GetLicenseByID(ctx, missing)
	require.ErrorIs(t, err, domain.ErrLicenseNotFound)
}
