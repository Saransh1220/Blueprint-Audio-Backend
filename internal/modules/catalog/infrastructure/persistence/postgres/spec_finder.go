package postgres

// Additional SpecFinder methods for other modules
import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
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
