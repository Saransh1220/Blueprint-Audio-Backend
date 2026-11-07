package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

// saveLicensePricesTx is shared by creation, editing and explicit price updates.
// The caller's transaction keeps the option and all its prices atomic.
func saveLicensePricesTx(ctx context.Context, tx *sqlx.Tx, licenseID uuid.UUID, prices []money.StoredPrice) error {
	seen := make(map[string]bool)
	for i := range prices {
		if err := prices[i].Validate(); err != nil {
			return err
		}
		if seen[prices[i].Currency] {
			return fmt.Errorf("duplicate price currency %s", prices[i].Currency)
		}
		seen[prices[i].Currency] = true
	}
	const query = `INSERT INTO license_option_prices
		(id, license_option_id, currency, amount_minor, amount_major, source, revision, is_active, fx_rate_snapshot, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,1,$7,$8,NOW(),NOW())
		ON CONFLICT (license_option_id, currency) DO UPDATE SET
		amount_minor=EXCLUDED.amount_minor, amount_major=EXCLUDED.amount_major,
		source=EXCLUDED.source, is_active=EXCLUDED.is_active, fx_rate_snapshot=EXCLUDED.fx_rate_snapshot,
		revision=license_option_prices.revision + CASE WHEN
			license_option_prices.amount_minor IS DISTINCT FROM EXCLUDED.amount_minor OR
			license_option_prices.is_active IS DISTINCT FROM EXCLUDED.is_active THEN 1 ELSE 0 END,
		updated_at=NOW()
		WHERE license_option_prices.revision=$9 AND license_option_prices.id=EXCLUDED.id
		RETURNING id, revision, amount_major`
	for i := range prices {
		p := &prices[i]
		if p.ID == uuid.Nil {
			id, err := uuid.NewV7()
			if err != nil {
				return err
			}
			p.ID = id
		}
		p.LicenseOptionID = licenseID
		if p.Source == "" {
			p.Source = "owner"
		}
		err := tx.QueryRowxContext(ctx, query, p.ID, licenseID, p.Currency, p.AmountMinor, p.AmountMajor, p.Source, p.IsActive, p.FxRateSnapshot, p.Revision).Scan(&p.ID, &p.Revision, &p.AmountMajor)
		if errors.Is(err, sql.ErrNoRows) {
			return money.ErrStalePrice
		}
		if err != nil {
			return err
		}
	}
	return nil
}
