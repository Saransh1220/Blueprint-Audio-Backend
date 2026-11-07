package application

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/saransh1220/blueprint-audio/internal/modules/fx/domain"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

type Config struct {
	Enabled           bool
	RefreshInterval   time.Duration
	StaleAfter        time.Duration
	MaxAge            time.Duration
	MaxDailyMove      float64
	ReportingCurrency string
}

type Service struct {
	repo      domain.RateRepository
	refresher *Refresher
	cfg       Config
}

func NewService(repo domain.RateRepository, refresher *Refresher, cfg Config) *Service {
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = 36 * time.Hour
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = 7 * 24 * time.Hour
	}
	if cfg.ReportingCurrency == "" {
		cfg.ReportingCurrency = "INR"
	}
	return &Service{
		repo:      repo,
		refresher: refresher,
		cfg:       cfg,
	}
}

func (s *Service) GetLatestRate(ctx context.Context, base, quote string) (*domain.Rate, error) {
	rate, err := s.repo.Latest(ctx, base, quote)
	if err == nil {
		now := time.Now().UTC()
		s.decorateFlags(rate, now)
		return rate, nil
	}
	if !errors.Is(err, domain.ErrNoRate) {
		return nil, err
	}

	// Try inverse pair if direct pair is missing
	invRate, invErr := s.repo.Latest(ctx, quote, base)
	if invErr != nil {
		return nil, domain.ErrNoRate
	}
	now := time.Now().UTC()
	s.decorateFlags(invRate, now)

	rat := invRate.GetBigRat()
	if rat == nil {
		parsed, parseErr := money.ParseRateDecimal(invRate.RateDecimal)
		if parseErr != nil {
			return nil, parseErr
		}
		rat = parsed
	}
	invertedRat := new(big.Rat).Inv(rat)
	invertedDecimal := money.FormatRateDecimal(invertedRat)

	return &domain.Rate{
		ID:          invRate.ID,
		Base:        base,
		Quote:       quote,
		Rate:        invertedRat,
		RateDecimal: invertedDecimal,
		Source:      invRate.Source,
		AsOf:        invRate.AsOf,
		FetchedAt:   invRate.FetchedAt,
		Status:      invRate.Status,
		Stale:       invRate.Stale,
		Unusable:    invRate.Unusable,
	}, nil
}

func (s *Service) GetHistory(ctx context.Context, filter domain.HistoryFilter) ([]*domain.Rate, int, error) {
	rates, total, err := s.repo.History(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	now := time.Now().UTC()
	for _, rate := range rates {
		s.decorateFlags(rate, now)
	}
	return rates, total, nil
}

func (s *Service) GetLatestPerPair(ctx context.Context) ([]*domain.Rate, error) {
	rates, err := s.repo.LatestPerPair(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for _, rate := range rates {
		s.decorateFlags(rate, now)
	}
	return rates, nil
}

func (s *Service) Refresh(ctx context.Context, force bool) (*RefreshResult, error) {
	if s.refresher == nil {
		return nil, domain.ErrNoRate
	}
	return s.refresher.Refresh(ctx, force)
}

func (s *Service) decorateFlags(rate *domain.Rate, now time.Time) {
	if rate == nil {
		return
	}
	rate.Stale = now.Sub(rate.FetchedAt) > s.cfg.StaleAfter
	rate.Unusable = now.Sub(rate.AsOf) > s.cfg.MaxAge
}
