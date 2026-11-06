package postgres

// Additional SpecFinder methods for other modules
import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

// FindByID implements domain.SpecFinder - public alias for GetByID
func (r *PgSpecRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Spec, error) {
	return r.GetByID(ctx, id)
}

// FindWithLicenses implements domain.SpecFinder
func (r *PgSpecRepository) FindWithLicenses(ctx context.Context, id uuid.UUID) (*domain.Spec, error) {
	return r.GetByID(ctx, id) // Already includes licenses
}

// Exists implements domain.SpecFinder
func (r *PgSpecRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM specs WHERE id = $1)`
	err := r.db.GetContext(ctx, &exists, query, id)
	return exists, err
}

// GetLicenseByID implements domain.SpecFinder
func (r *PgSpecRepository) GetLicenseByID(ctx context.Context, licenseID uuid.UUID) (*domain.LicenseOption, error) {
	license := &domain.LicenseOption{}
	query := `SELECT * FROM license_options WHERE id = $1 AND is_deleted = FALSE`
	err := r.db.GetContext(ctx, license, query, licenseID)
	if err == sql.ErrNoRows {
		return nil, domain.ErrLicenseNotFound
	}
	if err != nil {
		return nil, err
	}
	return license, nil
}

// GetLicensePrices implements domain.SpecFinder
func (r *PgSpecRepository) GetLicensePrices(ctx context.Context, licenseID uuid.UUID) ([]money.StoredPrice, error) {
	query := `
		SELECT id, license_option_id, currency, amount_minor, amount_major, source, fx_rate_snapshot, revision, is_active, created_at, updated_at
		FROM license_option_prices
		WHERE license_option_id = $1 AND is_active = TRUE
		ORDER BY currency ASC
	`
	var prices []money.StoredPrice
	err := r.db.SelectContext(ctx, &prices, query, licenseID)
	if err != nil {
		return nil, err
	}
	return prices, nil
}

// UpsertLicensePrices updates or inserts explicit prices for a license option.
func (r *PgSpecRepository) UpsertLicensePrices(ctx context.Context, licenseID uuid.UUID, prices []money.StoredPrice) error {
	if len(prices) == 0 {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := saveLicensePricesTx(ctx, tx, licenseID, prices); err != nil {
		return err
	}
	return tx.Commit()
}
