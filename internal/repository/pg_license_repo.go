package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/domain"
)

type pgLicenseRepository struct {
	db *sqlx.DB
}

func NewLicenseRepository(db *sqlx.DB) domain.LicenseRepository {
	return &pgLicenseRepository{db: db}
}

func (r *pgLicenseRepository) Create(ctx context.Context, license *domain.License) error {
	if license.ID == uuid.Nil {
		license.ID = uuid.New()
	}
	if license.CreatedAt.IsZero() {
		license.CreatedAt = time.Now()
	}
	license.UpdatedAt = time.Now()
	license.IssuedAt = time.Now()

	query := `
		INSERT INTO licenses (
			id, order_id, user_id, spec_id, license_option_id,
			license_type, purchase_price, license_key,
			is_active, is_revoked, downloads_count,
			issued_at, created_at, updated_at
		) VALUES (
			:id, :order_id, :user_id, :spec_id, :license_option_id,
			:license_type, :purchase_price, :license_key,
			:is_active, :is_revoked, :downloads_count,
			:issued_at, :created_at, :updated_at
		)`

	_, err := r.db.NamedExecContext(ctx, query, license)
	return err
}

func (r *pgLicenseRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.License, error) {
	license := &domain.License{}
	query := `SELECT * FROM licenses WHERE id = $1`
	err := r.db.GetContext(ctx, license, query, id)
	return license, err
}

func (r *pgLicenseRepository) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.License, error) {
	license := &domain.License{}
	query := `SELECT * FROM licenses WHERE order_id = $1`
	err := r.db.GetContext(ctx, license, query, orderID)
	return license, err
}

func (r *pgLicenseRepository) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.License, error) {
	var licenses []domain.License
	query := `SELECT * FROM licenses WHERE user_id = $1 AND is_active = true ORDER BY issued_at DESC LIMIT $2 OFFSET $3`
	err := r.db.SelectContext(ctx, &licenses, query, userID, limit, offset)
	return licenses, err
}

func (r *pgLicenseRepository) IncrementDownloads(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE licenses 
		SET downloads_count = downloads_count + 1,
		    last_downloaded_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *pgLicenseRepository) Revoke(ctx context.Context, id uuid.UUID, reason string) error {
	query := `
		UPDATE licenses 
		SET is_revoked = true,
		    revoked_reason = $1,
		    revoked_at = NOW(),
		    updated_at = NOW()
		WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, reason, id)
	return err
}
