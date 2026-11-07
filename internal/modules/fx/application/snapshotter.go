package application

import (
	"context"
	"log"
	"time"

	"github.com/saransh1220/blueprint-audio/internal/modules/fx/domain"
	paymentDomain "github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
)

type Snapshotter struct {
	repo domain.RateRepository
	cfg  Config
}

func NewSnapshotter(repo domain.RateRepository, cfg Config) *Snapshotter {
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = 7 * 24 * time.Hour
	}
	if cfg.ReportingCurrency == "" {
		cfg.ReportingCurrency = "INR"
	}
	return &Snapshotter{
		repo: repo,
		cfg:  cfg,
	}
}

// Snapshot obtains the reporting currency FX rate snapshot for an order.
// Crucial invariant: failure, stale/unusable rate or provider outage must NEVER return an error
// that blocks the charge. When unusable or unavailable, it returns a nil snapshot and nil error.
func (s *Snapshotter) Snapshot(ctx context.Context, orderCurrency string) (*paymentDomain.OrderFXSnapshot, error) {
	reportingCurrency := s.cfg.ReportingCurrency
	now := time.Now().UTC()

	// 1. Same currency as reporting currency: identity rate 1.00000000, no DB lookup, no RateID
	if orderCurrency == reportingCurrency {
		identityRate := "1.00000000"
		return &paymentDomain.OrderFXSnapshot{
			ReportCurrency:    reportingCurrency,
			ReportFXRate:      &identityRate,
			ReportFXRateID:    nil,
			ReportFXAsOf:      &now,
			ReportFXFetchedAt: &now,
		}, nil
	}

	// 2. Different currency: lookup latest accepted rate for orderCurrency -> reportingCurrency
	rate, err := s.repo.Latest(ctx, orderCurrency, reportingCurrency)
	if err != nil {
		log.Printf("[WARN] fx rate snapshot unavailable for pair %s/%s: %v", orderCurrency, reportingCurrency, err)
		return nil, nil // non-blocking by design
	}

	// 3. Verify rate is accepted and usable (now - as_of <= maxAge)
	if rate.Status != domain.RateStatusAccepted {
		log.Printf("[WARN] fx rate for pair %s/%s is quarantined (%s), skipping snapshot", orderCurrency, reportingCurrency, rate.Status)
		return nil, nil // non-blocking by design
	}

	if now.Sub(rate.AsOf) > s.cfg.MaxAge {
		log.Printf("[WARN] fx rate for pair %s/%s is unusable (as_of=%s older than %s), skipping snapshot", orderCurrency, reportingCurrency, rate.AsOf, s.cfg.MaxAge)
		return nil, nil // non-blocking by design
	}

	return &paymentDomain.OrderFXSnapshot{
		ReportCurrency:    reportingCurrency,
		ReportFXRate:      &rate.RateDecimal,
		ReportFXRateID:    &rate.ID,
		ReportFXAsOf:      &rate.AsOf,
		ReportFXFetchedAt: &rate.FetchedAt,
	}, nil
}
