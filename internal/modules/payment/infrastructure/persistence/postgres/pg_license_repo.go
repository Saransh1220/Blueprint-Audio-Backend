package postgres

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
)

type PgLicenseRepository struct {
	db *sqlx.DB
}

func NewLicenseRepository(db *sqlx.DB) domain.LicenseRepository {
	return &PgLicenseRepository{db: db}
}

func (r *PgLicenseRepository) Create(ctx context.Context, license *domain.License) error {
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

func (r *PgLicenseRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.License, error) {
	license := &domain.License{}
	query := `SELECT * FROM licenses WHERE id = $1`
	err := r.db.GetContext(ctx, license, query, id)
	return license, err
}

func (r *PgLicenseRepository) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.License, error) {
	license := &domain.License{}
	query := `SELECT * FROM licenses WHERE order_id = $1`
	err := r.db.GetContext(ctx, license, query, orderID)
	return license, err
}

func (r *PgLicenseRepository) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int, search, licenseType string) ([]domain.License, int, error) {
	var results []struct {
		domain.License
		TotalCount int `db:"total_count"`
	}
	query := `
		SELECT l.*, s.title as spec_title, s.image_url as spec_image, COUNT(*) OVER() as total_count
		FROM licenses l
		JOIN specs s ON l.spec_id = s.id
		WHERE l.user_id = $1 AND l.is_active = true`

	args := []interface{}{userID}
	argIdx := 2 // $1 is used

	if search != "" {
		query += " AND s.title ILIKE $" + strconv.Itoa(argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}

	if licenseType != "" {
		query += " AND l.license_type = $" + strconv.Itoa(argIdx)
		args = append(args, licenseType)
		argIdx++
	}

	query += " ORDER BY l.issued_at DESC LIMIT $" + strconv.Itoa(argIdx) + " OFFSET $" + strconv.Itoa(argIdx+1)
	args = append(args, limit, offset)

	err := r.db.SelectContext(ctx, &results, query, args...)
	if err != nil {
		return nil, 0, err
	}

	if len(results) == 0 {
		return []domain.License{}, 0, nil
	}

	total := results[0].TotalCount
	licenses := make([]domain.License, len(results))
	for i, res := range results {
		licenses[i] = res.License
	}

	return licenses, total, nil
}

func (r *PgLicenseRepository) IncrementDownloads(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE licenses 
		SET downloads_count = downloads_count + 1,
		    last_downloaded_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *PgLicenseRepository) Revoke(ctx context.Context, id uuid.UUID, reason string) error {
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
