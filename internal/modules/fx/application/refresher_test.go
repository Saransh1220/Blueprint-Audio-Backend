package application_test

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/saransh1220/blueprint-audio/internal/modules/fx/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/fx/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockLockManager struct {
	mock.Mock
}

func (m *mockLockManager) TryLock(ctx context.Context, key string) (func() error, bool, error) {
	args := m.Called(ctx, key)
	fn := args.Get(0)
	if fn == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return fn.(func() error), args.Bool(1), args.Error(2)
}

type mockRateProvider struct {
	mock.Mock
}

func (m *mockRateProvider) Name() string {
	return "mock-provider"
}

func (m *mockRateProvider) Fetch(ctx context.Context, base string, quotes []string) (*domain.ProviderRates, error) {
	args := m.Called(ctx, base, quotes)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ProviderRates), args.Error(1)
}

func TestRefresher_Refresh_SkippedWhenNotDue(t *testing.T) {
	repo := new(mockRateRepo)
	lockMgr := new(mockLockManager)
	prov := new(mockRateProvider)
	cfg := application.Config{RefreshInterval: 24 * time.Hour}
	refresher := application.NewRefresher(repo, lockMgr, prov, cfg)

	ctx := context.Background()
	now := time.Now().UTC()

	// All pairs fresh (fetched 1h ago)
	rates := []*domain.Rate{
		{Base: "USD", Quote: "INR", FetchedAt: now.Add(-1 * time.Hour)},
		{Base: "INR", Quote: "USD", FetchedAt: now.Add(-1 * time.Hour)},
	}
	repo.On("LatestPerPair", ctx).Return(rates, nil).Once()

	res, err := refresher.Refresh(ctx, false)
	require.NoError(t, err)
	assert.True(t, res.Skipped)
	assert.Equal(t, "rates are up to date", res.Reason)

	repo.AssertExpectations(t)
}

func TestRefresher_Refresh_LockBusy(t *testing.T) {
	repo := new(mockRateRepo)
	lockMgr := new(mockLockManager)
	prov := new(mockRateProvider)
	cfg := application.Config{RefreshInterval: 24 * time.Hour}
	refresher := application.NewRefresher(repo, lockMgr, prov, cfg)

	ctx := context.Background()

	// Empty repo so it is due
	repo.On("LatestPerPair", ctx).Return([]*domain.Rate{}, nil).Once()

	// Lock busy
	lockMgr.On("TryLock", ctx, "fx_refresh").Return(nil, false, nil).Once()

	res, err := refresher.Refresh(ctx, false)
	require.NoError(t, err)
	assert.True(t, res.Skipped)
	assert.Equal(t, "refresh already in progress", res.Reason)

	// Forced refresh when lock is busy returns ErrLockNotAcquired
	lockMgr.On("TryLock", ctx, "fx_refresh").Return(nil, false, nil).Once()
	_, err = refresher.Refresh(ctx, true)
	assert.ErrorIs(t, err, domain.ErrLockNotAcquired)

	lockMgr.AssertExpectations(t)
}

func TestRefresher_Refresh_Cooldown(t *testing.T) {
	repo := new(mockRateRepo)
	lockMgr := new(mockLockManager)
	prov := new(mockRateProvider)
	cfg := application.Config{RefreshInterval: 24 * time.Hour, MaxDailyMove: 0.20}
	refresher := application.NewRefresher(repo, lockMgr, prov, cfg)

	ctx := context.Background()
	now := time.Now().UTC()

	unlockCalled := false
	unlock := func() error {
		unlockCalled = true
		return nil
	}

	lockMgr.On("TryLock", ctx, "fx_refresh").Return(unlock, true, nil).Once()

	prov.On("Fetch", ctx, "INR", []string{"USD"}).Return(&domain.ProviderRates{
		Base:   "INR",
		AsOf:   now,
		Rates:  map[string]*big.Rat{"USD": big.NewRat(12, 1000)},
		Source: "mock-provider",
	}, nil).Once()

	prov.On("Fetch", ctx, "USD", []string{"INR"}).Return(&domain.ProviderRates{
		Base:   "USD",
		AsOf:   now,
		Rates:  map[string]*big.Rat{"INR": big.NewRat(8312, 100)},
		Source: "mock-provider",
	}, nil).Once()

	repo.On("Latest", ctx, "INR", "USD").Return(nil, domain.ErrNoRate).Once()
	repo.On("Latest", ctx, "USD", "INR").Return(nil, domain.ErrNoRate).Once()

	repo.On("AppendBatch", ctx, mock.MatchedBy(func(batch []*domain.Rate) bool {
		return len(batch) == 2
	})).Return(nil).Once()

	// First force succeeds
	res, err := refresher.Refresh(ctx, true)
	require.NoError(t, err)
	assert.Equal(t, 2, res.RatesUpdated)
	assert.True(t, unlockCalled)

	// Immediate second force hits cooldown
	_, err = refresher.Refresh(ctx, true)
	assert.ErrorIs(t, err, domain.ErrCooldownActive)
}

func TestRefresher_Refresh_QuarantineOnExcessiveMove(t *testing.T) {
	repo := new(mockRateRepo)
	lockMgr := new(mockLockManager)
	prov := new(mockRateProvider)
	cfg := application.Config{RefreshInterval: 24 * time.Hour, MaxDailyMove: 0.20}
	refresher := application.NewRefresher(repo, lockMgr, prov, cfg)

	ctx := context.Background()
	now := time.Now().UTC()

	unlock := func() error { return nil }
	lockMgr.On("TryLock", ctx, "fx_refresh").Return(unlock, true, nil).Once()

	// INR -> USD is normal
	prov.On("Fetch", ctx, "INR", []string{"USD"}).Return(&domain.ProviderRates{
		Base:   "INR",
		AsOf:   now,
		Rates:  map[string]*big.Rat{"USD": big.NewRat(12, 1000)},
		Source: "mock-provider",
	}, nil).Once()

	// USD -> INR jumps 50% from 80 to 120!
	prov.On("Fetch", ctx, "USD", []string{"INR"}).Return(&domain.ProviderRates{
		Base:   "USD",
		AsOf:   now,
		Rates:  map[string]*big.Rat{"INR": big.NewRat(120, 1)},
		Source: "mock-provider",
	}, nil).Once()

	prevRate := &domain.Rate{
		Base:  "USD",
		Quote: "INR",
		Rate:  big.NewRat(80, 1),
	}
	repo.On("Latest", ctx, "INR", "USD").Return(nil, domain.ErrNoRate).Once()
	repo.On("Latest", ctx, "USD", "INR").Return(prevRate, nil).Once()

	var capturedBatch []*domain.Rate
	repo.On("AppendBatch", ctx, mock.MatchedBy(func(batch []*domain.Rate) bool {
		capturedBatch = batch
		return true
	})).Return(nil).Once()

	res, err := refresher.Refresh(ctx, true)
	require.NoError(t, err)
	assert.Equal(t, 2, res.RatesUpdated)

	// USD -> INR must be quarantined!
	var usdInr *domain.Rate
	for _, r := range capturedBatch {
		if r.Base == "USD" && r.Quote == "INR" {
			usdInr = r
		}
	}
	require.NotNil(t, usdInr)
	assert.Equal(t, domain.RateStatusQuarantined, usdInr.Status)
}

func TestRefresher_Refresh_ProviderFailureWithRetries(t *testing.T) {
	repo := new(mockRateRepo)
	lockMgr := new(mockLockManager)
	prov := new(mockRateProvider)
	cfg := application.Config{RefreshInterval: 24 * time.Hour}
	refresher := application.NewRefresher(repo, lockMgr, prov, cfg)

	ctx := context.Background()
	unlock := func() error { return nil }
	lockMgr.On("TryLock", ctx, "fx_refresh").Return(unlock, true, nil).Once()

	prov.On("Fetch", ctx, "INR", []string{"USD"}).Return(nil, errors.New("provider down")).Times(3)

	_, err := refresher.Refresh(ctx, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch fx rates")

	prov.AssertExpectations(t)
}

func TestRefresher_StartRefresher_ContextCancel(t *testing.T) {
	repo := new(mockRateRepo)
	lockMgr := new(mockLockManager)
	prov := new(mockRateProvider)
	cfg := application.Config{RefreshInterval: 24 * time.Hour}
	refresher := application.NewRefresher(repo, lockMgr, prov, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	refresher.StartRefresher(ctx, 10*time.Millisecond)
}
