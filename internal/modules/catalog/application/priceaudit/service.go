package priceaudit

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

type Option struct {
	ID       uuid.UUID `db:"id"`
	SpecID   uuid.UUID `db:"spec_id"`
	Name     string    `db:"name"`
	Price    float64   `db:"price"`
	Currency string    `db:"price_currency"`
}

type Change struct {
	OptionID    uuid.UUID          `json:"option_id"`
	Currency    string             `json:"currency"`
	AmountMinor int                `json:"amount_minor"`
	Before      *money.StoredPrice `json:"before"`
}

// A reviewed manifest is an immutable proposal, not permission to overwrite
// intervening owner edits. The repository rechecks both legacy and price rows.
type Manifest struct {
	Version   int       `json:"version"`
	BatchID   uuid.UUID `json:"batch_id"`
	CreatedAt time.Time `json:"created_at"`
	Changes   []Change  `json:"changes"`
}

type Repository interface {
	Read(context.Context) ([]Option, []money.StoredPrice, error)
	Apply(context.Context, Manifest) (int, error)
	Rollback(context.Context, uuid.UUID) (int, error)
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) Audit(ctx context.Context) (AuditSummary, Manifest, error) {
	options, prices, err := s.repo.Read(ctx)
	if err != nil {
		return AuditSummary{}, Manifest{}, err
	}
	byOption := make(map[uuid.UUID][]money.StoredPrice)
	for _, p := range prices {
		byOption[p.LicenseOptionID] = append(byOption[p.LicenseOptionID], p)
	}
	m := Manifest{Version: 1, BatchID: uuid.New(), CreatedAt: time.Now().UTC(), Changes: []Change{}}
	rows := make([]OptionAuditRow, 0, len(options))
	for _, opt := range options {
		row := AuditOption(opt.ID, opt.SpecID, opt.Name, opt.Price, opt.Currency, byOption[opt.ID])
		rows = append(rows, row)
		for _, c := range row.Classifications {
			if c == ClassMissingNative || c == ClassStaleLegacyMismatch {
				minor, err := QuantizeDecimal(row.NativePrice)
				if err != nil {
					return AuditSummary{}, Manifest{}, err
				}
				m.Changes = append(m.Changes, Change{OptionID: opt.ID, Currency: row.NativeCurrency, AmountMinor: minor, Before: row.ExpectedBefore})
				break
			}
		}
	}
	return CompileAuditSummary(rows), m, nil
}

func (s *Service) Apply(ctx context.Context, m Manifest) (int, error) {
	if m.Version != 1 || m.BatchID == uuid.Nil || m.CreatedAt.IsZero() {
		return 0, fmt.Errorf("invalid manifest identity/version")
	}
	seen := make(map[string]bool)
	for _, c := range m.Changes {
		p := money.StoredPrice{Currency: c.Currency, AmountMinor: c.AmountMinor}
		if err := p.Validate(); err != nil {
			return 0, err
		}
		if c.OptionID == uuid.Nil || p.Currency != c.Currency {
			return 0, fmt.Errorf("invalid manifest option/currency")
		}
		key := c.OptionID.String() + c.Currency
		if seen[key] {
			return 0, fmt.Errorf("duplicate manifest change")
		}
		seen[key] = true
		if c.Before != nil && (c.Before.LicenseOptionID != c.OptionID || c.Before.Currency != c.Currency || !c.Before.IsActive || c.Before.Source != "legacy_price" || c.Before.Revision > 1) {
			return 0, fmt.Errorf("manifest cannot overwrite manual/inactive price")
		}
	}
	return s.repo.Apply(ctx, m)
}

func (s *Service) Rollback(ctx context.Context, batch uuid.UUID) (int, error) {
	if batch == uuid.Nil {
		return 0, fmt.Errorf("invalid batch ID")
	}
	return s.repo.Rollback(ctx, batch)
}
