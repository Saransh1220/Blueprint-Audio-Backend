package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/application/priceaudit"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

type PriceAuditRepository struct{ db *sqlx.DB }

func NewPriceAuditRepository(db *sqlx.DB) *PriceAuditRepository { return &PriceAuditRepository{db: db} }

const priceColumns = `id,license_option_id,currency,amount_minor,amount_major,source,fx_rate_snapshot,revision,is_active,created_at,updated_at`

func (r *PriceAuditRepository) Read(ctx context.Context) ([]priceaudit.Option, []money.StoredPrice, error) {
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	var options []priceaudit.Option
	var prices []money.StoredPrice
	if err = tx.SelectContext(ctx, &options, `SELECT id,spec_id,name,price,price_currency FROM license_options WHERE is_deleted=FALSE ORDER BY id`); err != nil {
		return nil, nil, err
	}
	if err = tx.SelectContext(ctx, &prices, `SELECT `+priceColumns+` FROM license_option_prices ORDER BY license_option_id,currency`); err != nil {
		return nil, nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, nil, err
	}
	return options, prices, nil
}

func (r *PriceAuditRepository) Apply(ctx context.Context, m priceaudit.Manifest) (int, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	body, err := json.Marshal(m)
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO price_reconciliation_batches(id,manifest) VALUES($1,$2) ON CONFLICT DO NOTHING`, m.BatchID, body)
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if count == 0 {
		var existing struct {
			Manifest   []byte       `db:"manifest"`
			RolledBack sql.NullTime `db:"rolled_back_at"`
		}
		if err = tx.GetContext(ctx, &existing, `SELECT manifest,rolled_back_at FROM price_reconciliation_batches WHERE id=$1 FOR UPDATE`, m.BatchID); err != nil {
			return 0, err
		}
		var prior priceaudit.Manifest
		if err = json.Unmarshal(existing.Manifest, &prior); err != nil {
			return 0, err
		}
		if !reflect.DeepEqual(prior, m) || existing.RolledBack.Valid {
			return 0, fmt.Errorf("batch ID already used by a different or rolled-back manifest")
		}
		return 0, tx.Commit()
	}
	for _, change := range m.Changes {
		var native struct {
			Price    float64 `db:"price"`
			Currency string  `db:"price_currency"`
		}
		if err = tx.GetContext(ctx, &native, `SELECT price,price_currency FROM license_options WHERE id=$1 AND is_deleted=FALSE FOR UPDATE`, change.OptionID); err != nil {
			return 0, err
		}
		minor, e := money.MajorToMinor(native.Price)
		if e != nil {
			return 0, e
		}
		currency := strings.ToUpper(strings.TrimSpace(native.Currency))
		if currency == "" {
			currency = money.CurrencyINR
		}
		if minor != change.AmountMinor || currency != change.Currency {
			return 0, fmt.Errorf("legacy price changed since reviewed manifest")
		}
		var current money.StoredPrice
		err = tx.GetContext(ctx, &current, `SELECT `+priceColumns+` FROM license_option_prices WHERE license_option_id=$1 AND currency=$2 FOR UPDATE`, change.OptionID, change.Currency)
		if change.Before == nil {
			if !errors.Is(err, sql.ErrNoRows) {
				if err != nil {
					return 0, err
				}
				return 0, money.ErrStalePrice
			}
		} else {
			if err != nil {
				return 0, err
			}
			before := change.Before
			if current.ID != before.ID || current.Revision != before.Revision || current.AmountMinor != before.AmountMinor || current.Source != before.Source || !current.IsActive || !current.UpdatedAt.Equal(before.UpdatedAt) {
				return 0, money.ErrStalePrice
			}
		}
		var beforeJSON any
		if change.Before != nil {
			b, e := json.Marshal(current)
			if e != nil {
				return 0, e
			}
			beforeJSON = b
		}
		p := money.StoredPrice{LicenseOptionID: change.OptionID, Currency: change.Currency, AmountMinor: change.AmountMinor, IsActive: true, Source: "reconciliation", Revision: 1}
		if change.Before != nil {
			p.ID = current.ID
			p.Revision = current.Revision
			p.FxRateSnapshot = current.FxRateSnapshot
		}
		if err = saveLicensePricesTx(ctx, tx, change.OptionID, []money.StoredPrice{p}); err != nil {
			return 0, err
		}
		var after money.StoredPrice
		if err = tx.GetContext(ctx, &after, `SELECT `+priceColumns+` FROM license_option_prices WHERE license_option_id=$1 AND currency=$2`, change.OptionID, change.Currency); err != nil {
			return 0, err
		}
		afterJSON, e := json.Marshal(after)
		if e != nil {
			return 0, e
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO price_reconciliation_changes(batch_id,price_id,before_row,after_row) VALUES($1,$2,$3,$4)`, m.BatchID, after.ID, beforeJSON, afterJSON); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(m.Changes), nil
}

func (r *PriceAuditRepository) Rollback(ctx context.Context, batch uuid.UUID) (int, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var rolled sql.NullTime
	if err = tx.GetContext(ctx, &rolled, `SELECT rolled_back_at FROM price_reconciliation_batches WHERE id=$1 FOR UPDATE`, batch); err != nil {
		return 0, err
	}
	if rolled.Valid {
		return 0, tx.Commit()
	}
	var changes []struct {
		PriceID uuid.UUID `db:"price_id"`
		Before  []byte    `db:"before_row"`
		After   []byte    `db:"after_row"`
	}
	if err = tx.SelectContext(ctx, &changes, `SELECT price_id,before_row,after_row FROM price_reconciliation_changes WHERE batch_id=$1 ORDER BY price_id`, batch); err != nil {
		return 0, err
	}
	for _, change := range changes {
		var after money.StoredPrice
		if err = json.Unmarshal(change.After, &after); err != nil {
			return 0, err
		}
		var result sql.Result
		if len(change.Before) == 0 || string(change.Before) == "null" {
			result, err = tx.ExecContext(ctx, `DELETE FROM license_option_prices WHERE id=$1 AND revision=$2 AND amount_minor=$3 AND is_active=$4 AND updated_at=$5`, change.PriceID, after.Revision, after.AmountMinor, after.IsActive, after.UpdatedAt)
		} else {
			var before money.StoredPrice
			if err = json.Unmarshal(change.Before, &before); err != nil {
				return 0, err
			}
			result, err = tx.ExecContext(ctx, `UPDATE license_option_prices SET amount_minor=$1,amount_major=$2,source=$3,fx_rate_snapshot=$4,is_active=$5,revision=revision+1,updated_at=NOW()
			 WHERE id=$6 AND revision=$7 AND amount_minor=$8 AND is_active=$9 AND updated_at=$10`, before.AmountMinor, before.AmountMajor, before.Source, before.FxRateSnapshot, before.IsActive, change.PriceID, after.Revision, after.AmountMinor, after.IsActive, after.UpdatedAt)
		}
		if err != nil {
			return 0, err
		}
		count, e := result.RowsAffected()
		if e != nil {
			return 0, e
		}
		if count != 1 {
			return 0, fmt.Errorf("rollback conflict: price %s was edited after apply", change.PriceID)
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE price_reconciliation_batches SET rolled_back_at=NOW() WHERE id=$1`, batch); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(changes), nil
}
