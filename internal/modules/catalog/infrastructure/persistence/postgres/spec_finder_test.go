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

	priceID := uuid.New()
	priceRows := sqlmock.NewRows([]string{"id", "license_option_id", "currency", "amount_minor", "amount_major", "source", "fx_rate_snapshot", "revision", "is_active", "created_at", "updated_at"}).
		AddRow(priceID, licenseID, "USD", 1500, 15.00, "owner", nil, 1, true, time.Now(), time.Now())
	mock.ExpectQuery(`SELECT .* FROM license_option_prices WHERE license_option_id = \$1 AND is_active = TRUE`).WithArgs(licenseID).WillReturnRows(priceRows)
	prices, err := repo.GetLicensePrices(ctx, licenseID)
	require.NoError(t, err)
	require.Len(t, prices, 1)
	assert.Equal(t, 1500, prices[0].AmountMinor)
	assert.Equal(t, "USD", prices[0].Currency)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO license_option_prices`).WillReturnRows(sqlmock.NewRows([]string{"id", "revision", "amount_major"}).AddRow(priceID, 1, 15.0))
	mock.ExpectCommit()

	err = repo.UpsertLicensePrices(ctx, licenseID, prices)
	require.NoError(t, err)
}

func TestSpecRepository_HydratesLicensePricesInGetByID(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer sqlDB.Close()
	db := sqlx.NewDb(sqlDB, "sqlmock")
	repo := postgres.NewSpecRepository(db)
	ctx := context.Background()
	specID := uuid.New()
	licenseID := uuid.New()
	priceID := uuid.New()

	specRows := sqlmock.NewRows([]string{"id", "producer_id", "producer_name", "title", "category", "type", "bpm", "key", "image_url", "preview_url", "wav_url", "stems_url", "base_price", "description", "duration", "free_mp3_enabled", "created_at", "updated_at", "is_deleted"}).
		AddRow(specID, uuid.New(), "p", "t", "beat", "wav", 120, "C", "img", "prev", nil, nil, 10.0, "d", 60, false, time.Now(), time.Now(), false)
	mock.ExpectQuery(`SELECT s\.\*, u\.display_name as producer_name`).WithArgs(specID).WillReturnRows(specRows)
	licRows := sqlmock.NewRows([]string{"id", "spec_id", "license_type", "name", "price", "features", "file_types", "is_deleted"}).
		AddRow(licenseID, specID, "Basic", "Basic", 10.0, "{}", "{}", false)
	mock.ExpectQuery(`SELECT \* FROM license_options WHERE spec_id = \$1 AND is_deleted = FALSE`).WithArgs(specID).WillReturnRows(licRows)

	priceRows := sqlmock.NewRows([]string{"id", "license_option_id", "currency", "amount_minor", "amount_major", "source", "fx_rate_snapshot", "revision", "is_active", "created_at", "updated_at"}).
		AddRow(priceID, licenseID, "USD", 1500, 15.00, "owner", nil, 1, true, time.Now(), time.Now())
	mock.ExpectQuery(`SELECT .+ FROM license_option_prices WHERE license_option_id IN`).WillReturnRows(priceRows)

	mock.ExpectQuery(`SELECT g\.\* FROM genres g JOIN spec_genres sg ON g\.id = sg\.genre_id WHERE sg\.spec_id = \$1`).WithArgs(specID).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "created_at"}))

	spec, err := repo.GetByID(ctx, specID)
	require.NoError(t, err)
	require.NotNil(t, spec)
	require.Len(t, spec.Licenses, 1)
	require.Len(t, spec.Licenses[0].Prices, 1)
	assert.Equal(t, priceID, spec.Licenses[0].Prices[0].ID)
	assert.Equal(t, "USD", spec.Licenses[0].Prices[0].Currency)
	assert.Equal(t, 1500, spec.Licenses[0].Prices[0].AmountMinor)
	assert.True(t, spec.Licenses[0].Prices[0].IsActive)
}
