package application

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/saransh1220/blueprint-audio/internal/modules/fx/domain"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

var (
	fxRefreshTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fx_refresh_total",
		Help: "Total number of FX refresh attempts by result (success, failure, skipped).",
	}, []string{"result"})

	fxRateAgeSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "fx_rate_age_seconds",
		Help: "Age of latest accepted FX rate in seconds.",
	}, []string{"base", "quote"})

	fxRateQuarantinedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fx_rate_quarantined_total",
		Help: "Total number of FX rates quarantined due to sanity band violations.",
	}, []string{"base", "quote"})
)

type RefreshResult struct {
	RatesUpdated int       `json:"rates_updated"`
	RefreshedAt  time.Time `json:"refreshed_at"`
	Skipped      bool      `json:"skipped,omitempty"`
	Reason       string    `json:"reason,omitempty"`
}

type Refresher struct {
	repo              domain.RateRepository
	lockMgr           domain.LockManager
	provider          domain.RateProvider
	cfg               Config
	mu                sync.Mutex
	lastManualRefresh time.Time
}

func NewRefresher(
	repo domain.RateRepository,
	lockMgr domain.LockManager,
	provider domain.RateProvider,
	cfg Config,
) *Refresher {
	if cfg.RefreshInterval <= 0 {
		cfg.RefreshInterval = 24 * time.Hour
	}
	if cfg.MaxDailyMove <= 0 {
		cfg.MaxDailyMove = 0.20 // 20%
	}
	return &Refresher{
		repo:     repo,
		lockMgr:  lockMgr,
		provider: provider,
		cfg:      cfg,
	}
}

// Refresh performs rate refresh with due-check, advisory locking, provider retry, and validation.
func (r *Refresher) Refresh(ctx context.Context, force bool) (*RefreshResult, error) {
	if r.provider == nil {
		return nil, errors.New("no fx rate provider configured")
	}

	now := time.Now().UTC()

	// 1. Manual cooldown check
	if force {
		r.mu.Lock()
		if !r.lastManualRefresh.IsZero() && now.Sub(r.lastManualRefresh) < time.Minute {
			r.mu.Unlock()
			return nil, domain.ErrCooldownActive
		}
		r.mu.Unlock()
	}

	// 2. Automated due-check
	if !force {
		latestList, err := r.repo.LatestPerPair(ctx)
		if err == nil && len(latestList) > 0 {
			allFresh := true
			for _, item := range latestList {
				if now.Sub(item.FetchedAt) >= r.cfg.RefreshInterval {
					allFresh = false
					break
				}
			}
			if allFresh {
				fxRefreshTotal.WithLabelValues("skipped").Inc()
				return &RefreshResult{
					Skipped: true,
					Reason:  "rates are up to date",
				}, nil
			}
		}
	}

	// 3. Advisory lock to prevent concurrent refresh across multiple instances
	if r.lockMgr != nil {
		unlock, acquired, err := r.lockMgr.TryLock(ctx, "fx_refresh")
		if err != nil {
			return nil, fmt.Errorf("failed to acquire advisory lock: %w", err)
		}
		if !acquired {
			if force {
				return nil, domain.ErrLockNotAcquired
			}
			fxRefreshTotal.WithLabelValues("skipped").Inc()
			return &RefreshResult{
				Skipped: true,
				Reason:  "refresh already in progress",
			}, nil
		}
		defer func() {
			if unlockErr := unlock(); unlockErr != nil {
				log.Printf("[WARN] failed to release fx refresh advisory lock: %v", unlockErr)
			}
		}()
	}

	// Double-check freshness after lock acquisition if not forced
	if !force {
		latestList, err := r.repo.LatestPerPair(ctx)
		if err == nil && len(latestList) > 0 {
			allFresh := true
			for _, item := range latestList {
				if time.Since(item.FetchedAt) >= r.cfg.RefreshInterval {
					allFresh = false
					break
				}
			}
			if allFresh {
				fxRefreshTotal.WithLabelValues("skipped").Inc()
				return &RefreshResult{
					Skipped: true,
					Reason:  "rates refreshed by another instance",
				}, nil
			}
		}
	}

	// 4. Fetch rates for all supported currency pairs
	currencies := money.SupportedCurrencies()
	var ratesToAppend []*domain.Rate

	for _, baseDef := range currencies {
		baseCode := baseDef.Code
		var quoteCodes []string
		for _, qDef := range currencies {
			if qDef.Code != baseCode {
				quoteCodes = append(quoteCodes, qDef.Code)
			}
		}
		if len(quoteCodes) == 0 {
			continue
		}

		// Retry logic for provider call: max 3 attempts with exponential backoff
		var provRates *domain.ProviderRates
		var fetchErr error
		backoff := 50 * time.Millisecond
		for attempt := 1; attempt <= 3; attempt++ {
			provRates, fetchErr = r.provider.Fetch(ctx, baseCode, quoteCodes)
			if fetchErr == nil {
				break
			}
			if attempt < 3 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(backoff):
					backoff *= 2
				}
			}
		}

		if fetchErr != nil {
			log.Printf("[ALERT][FX_REFRESH_FAILED] provider=%s base=%s err=%v", r.provider.Name(), baseCode, fetchErr)
			fxRefreshTotal.WithLabelValues("failure").Inc()
			return nil, fmt.Errorf("failed to fetch fx rates for %s: %w", baseCode, fetchErr)
		}

		for quoteCode, rateRat := range provRates.Rates {
			// Find previous accepted rate to evaluate sanity move
			prevAccepted, _ := r.repo.Latest(ctx, baseCode, quoteCode)
			status, valErr := domain.ValidateRate(rateRat, func() *domain.Rate {
				if prevAccepted != nil {
					return prevAccepted
				}
				return nil
			}().GetBigRat(), r.cfg.MaxDailyMove)

			if valErr != nil {
				log.Printf("[ALERT][FX_RATE_REJECTED] pair=%s/%s err=%v", baseCode, quoteCode, valErr)
				continue
			}

			if status == domain.RateStatusQuarantined {
				log.Printf("[ALERT][FX_RATE_SPIKE] pair=%s/%s rate=%s quarantined: exceeded max daily move of %.2f%%",
					baseCode, quoteCode, money.FormatRateDecimal(rateRat), r.cfg.MaxDailyMove*100)
				fxRateQuarantinedTotal.WithLabelValues(baseCode, quoteCode).Inc()
			}

			ratesToAppend = append(ratesToAppend, &domain.Rate{
				Base:        baseCode,
				Quote:       quoteCode,
				Rate:        rateRat,
				RateDecimal: money.FormatRateDecimal(rateRat),
				Source:      r.provider.Name(),
				AsOf:        provRates.AsOf,
				Status:      status,
			})
		}
	}

	if len(ratesToAppend) == 0 {
		return &RefreshResult{RatesUpdated: 0, RefreshedAt: now}, nil
	}

	// 5. Append batch to immutable rates history
	if err := r.repo.AppendBatch(ctx, ratesToAppend); err != nil {
		log.Printf("[ALERT][FX_REFRESH_FAILED] database append batch failed: %v", err)
		fxRefreshTotal.WithLabelValues("failure").Inc()
		return nil, fmt.Errorf("failed to persist fx rates: %w", err)
	}

	fxRefreshTotal.WithLabelValues("success").Inc()
	for _, rate := range ratesToAppend {
		if rate.Status == domain.RateStatusAccepted {
			fxRateAgeSeconds.WithLabelValues(rate.Base, rate.Quote).Set(0)
		}
	}

	if force {
		r.mu.Lock()
		r.lastManualRefresh = now
		r.mu.Unlock()
	}

	return &RefreshResult{
		RatesUpdated: len(ratesToAppend),
		RefreshedAt:  now,
	}, nil
}

// StartRefresher runs the background loop checking for due refresh until ctx is canceled.
func (r *Refresher) StartRefresher(ctx context.Context, checkInterval time.Duration) {
	if ctx.Err() != nil {
		return
	}
	if checkInterval <= 0 {
		checkInterval = 1 * time.Hour
	}

	// Initial refresh check on startup
	if _, err := r.Refresh(ctx, false); err != nil && ctx.Err() == nil {
		log.Printf("[FX Refresher] initial refresh check: %v", err)
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := r.Refresh(ctx, false); err != nil && ctx.Err() == nil {
				log.Printf("[FX Refresher] periodic refresh: %v", err)
			}
		}
	}
}
