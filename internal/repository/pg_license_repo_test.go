package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPGLicenseRepository_CreateGetUpdate(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := repository.NewLicenseRepository(db)
	ctx := context.Background()
	id := uuid.New()
	orderID := uuid.New()
	userID := uuid.New()
	specID := uuid.New()
	licenseOptionID := uuid.New()

	mock.ExpectExec("INSERT INTO licenses").
		WillReturnResult(sqlmock.NewResult(1, 1))
	err := repo.Create(ctx, &domain.License{
		ID:              id,
		OrderID:         orderID,
		UserID:          userID,
		SpecID:          specID,
		LicenseOptionID: licenseOptionID,
		LicenseType:     "Basic",
		PurchasePrice:   1000,
		LicenseKey:      "LIC-1",
		IsActive:        true,
	})
	require.NoError(t, err)

	rows := sqlmock.NewRows([]string{
		"id", "order_id", "user_id", "spec_id", "license_option_id", "license_type", "purchase_price",
		"license_key", "is_active", "is_revoked", "downloads_count", "issued_at", "created_at", "updated_at",
	}).AddRow(id, orderID, userID, specID, licenseOptionID, "Basic", 1000, "LIC-1", true, false, 0, time.Now(), time.Now(), time.Now())
	mock.ExpectQuery("SELECT \\* FROM licenses WHERE id = \\$1").WithArgs(id).WillReturnRows(rows)
	_, err = repo.GetByID(ctx, id)
	require.NoError(t, err)

	rows = sqlmock.NewRows([]string{
		"id", "order_id", "user_id", "spec_id", "license_option_id", "license_type", "purchase_price",
		"license_key", "is_active", "is_revoked", "downloads_count", "issued_at", "created_at", "updated_at",
	}).AddRow(id, orderID, userID, specID, licenseOptionID, "Basic", 1000, "LIC-1", true, false, 0, time.Now(), time.Now(), time.Now())
	mock.ExpectQuery("SELECT \\* FROM licenses WHERE order_id = \\$1").WithArgs(orderID).WillReturnRows(rows)
	_, err = repo.GetByOrderID(ctx, orderID)
	require.NoError(t, err)

	mock.ExpectExec("UPDATE licenses").
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.IncrementDownloads(ctx, id))

	mock.ExpectExec("UPDATE licenses").
		WithArgs("reason", id).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.Revoke(ctx, id, "reason"))
}

func TestPGLicenseRepository_ListByUser(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := repository.NewLicenseRepository(db)
	ctx := context.Background()
	userID := uuid.New()
	id := uuid.New()
	orderID := uuid.New()
	specID := uuid.New()
	optID := uuid.New()

	rows := sqlmock.NewRows([]string{
		"id", "order_id", "user_id", "spec_id", "license_option_id", "license_type", "purchase_price",
		"license_key", "is_active", "is_revoked", "downloads_count", "issued_at", "created_at", "updated_at",
		"spec_title", "spec_image", "total_count",
	}).AddRow(id, orderID, userID, specID, optID, "Basic", 1000, "LIC-1", true, false, 0, time.Now(), time.Now(), time.Now(), "Track", nil, 1)

	mock.ExpectQuery("SELECT l\\.\\*, s\\.title as spec_title, s\\.image_url as spec_image, COUNT\\(\\*\\) OVER\\(\\) as total_count").
		WillReturnRows(rows)
	licenses, total, err := repo.ListByUser(ctx, userID, 5, 0, "Tra", "Basic")
	require.NoError(t, err)
	assert.Len(t, licenses, 1)
	assert.Equal(t, 1, total)
}
