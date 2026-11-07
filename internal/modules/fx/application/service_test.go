package application_test

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/saransh1220/blueprint-audio/internal/modules/fx/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/fx/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockRateRepo struct {
	mock.Mock
}

func (m *mockRateRepo) Append(ctx context.Context, rate *domain.Rate) error {
	args := m.Called(ctx, rate)
	return args.Error(0)
}

func (m *mockRateRepo) AppendBatch(ctx context.Context, rates []*domain.Rate) error {
	args := m.Called(ctx, rates)
	return args.Error(0)
}

func (m *mockRateRepo) Latest(ctx context.Context, base, quote string) (*domain.Rate, error) {
	args := m.Called(ctx, base, quote)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Rate), args.Error(1)
}

func (m *mockRateRepo) History(ctx context.Context, filter domain.HistoryFilter) ([]*domain.Rate, int, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*domain.Rate), args.Int(1), args.Error(2)
}

func (m *mockRateRepo) LatestPerPair(ctx context.Context) ([]*domain.Rate, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Rate), args.Error(1)
}

func TestService_GetLatestRate_DecoratesFlags(t *testing.T) {
	repo := new(mockRateRepo)
	svc := application.NewService(repo, nil, application.Config{
		StaleAfter: 36 * time.Hour,
		MaxAge:     7 * 24 * time.Hour,
	})

	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Fresh rate (1 hour old)
	freshRate := &domain.Rate{
		ID:          1,
		Base:        "USD",
		Quote:       "INR",
		RateDecimal: "83.12000000",
		Rate:        big.NewRat(8312, 100),
		AsOf:        now.Add(-1 * time.Hour),
		FetchedAt:   now.Add(-1 * time.Hour),
		Status:      domain.RateStatusAccepted,
	}
	repo.On("Latest", ctx, "USD", "INR").Return(freshRate, nil).Once()

	rate, err := svc.GetLatestRate(ctx, "USD", "INR")
	require.NoError(t, err)
	assert.False(t, rate.Stale)
	assert.False(t, rate.Unusable)

	// 2. Stale rate (40 hours old fetch, but as_of is 40 hours <= 7d)
	staleRate := &domain.Rate{
		ID:          2,
		Base:        "USD",
		Quote:       "INR",
		RateDecimal: "83.12000000",
		Rate:        big.NewRat(8312, 100),
		AsOf:        now.Add(-40 * time.Hour),
		FetchedAt:   now.Add(-40 * time.Hour),
		Status:      domain.RateStatusAccepted,
	}
	repo.On("Latest", ctx, "USD", "INR").Return(staleRate, nil).Once()

	rate, err = svc.GetLatestRate(ctx, "USD", "INR")
	require.NoError(t, err)
	assert.True(t, rate.Stale)
	assert.False(t, rate.Unusable)

	// 3. Unusable rate (as_of is 8 days old)
	unusableRate := &domain.Rate{
		ID:          3,
		Base:        "USD",
		Quote:       "INR",
		RateDecimal: "83.12000000",
		Rate:        big.NewRat(8312, 100),
		AsOf:        now.Add(-8 * 24 * time.Hour),
		FetchedAt:   now.Add(-1 * time.Hour),
		Status:      domain.RateStatusAccepted,
	}
	repo.On("Latest", ctx, "USD", "INR").Return(unusableRate, nil).Once()

	rate, err = svc.GetLatestRate(ctx, "USD", "INR")
	require.NoError(t, err)
	assert.False(t, rate.Stale)
	assert.True(t, rate.Unusable)

	repo.AssertExpectations(t)
}

func TestService_GetHistory_And_LatestPerPair(t *testing.T) {
	repo := new(mockRateRepo)
	svc := application.NewService(repo, nil, application.Config{})
	ctx := context.Background()
	now := time.Now().UTC()

	rates := []*domain.Rate{
		{Base: "USD", Quote: "INR", AsOf: now, FetchedAt: now, Status: domain.RateStatusAccepted},
	}

	filter := domain.HistoryFilter{Base: "USD"}
	repo.On("History", ctx, filter).Return(rates, 1, nil).Once()
	res, total, err := svc.GetHistory(ctx, filter)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, res, 1)

	repo.On("LatestPerPair", ctx).Return(rates, nil).Once()
	pairs, err := svc.GetLatestPerPair(ctx)
	require.NoError(t, err)
	assert.Len(t, pairs, 1)

	repo.AssertExpectations(t)
}

func TestSnapshotter(t *testing.T) {
	repo := new(mockRateRepo)
	cfg := application.Config{
		ReportingCurrency: "INR",
		MaxAge:            7 * 24 * time.Hour,
	}
	snapshotter := application.NewSnapshotter(repo, cfg)
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("identity snapshot for same currency", func(t *testing.T) {
		snap, err := snapshotter.Snapshot(ctx, "INR")
		require.NoError(t, err)
		require.NotNil(t, snap)
		assert.Equal(t, "INR", snap.ReportCurrency)
		assert.Equal(t, "1.00000000", *snap.ReportFXRate)
		assert.Nil(t, snap.ReportFXRateID)
	})

	t.Run("usable snapshot for different currency", func(t *testing.T) {
		rateID := int64(100)
		rate := &domain.Rate{
			ID:          rateID,
			Base:        "USD",
			Quote:       "INR",
			RateDecimal: "83.12000000",
			AsOf:        now.Add(-2 * time.Hour),
			FetchedAt:   now.Add(-1 * time.Hour),
			Status:      domain.RateStatusAccepted,
		}
		repo.On("Latest", ctx, "USD", "INR").Return(rate, nil).Once()

		snap, err := snapshotter.Snapshot(ctx, "USD")
		require.NoError(t, err)
		require.NotNil(t, snap)
		assert.Equal(t, "INR", snap.ReportCurrency)
		assert.Equal(t, "83.12000000", *snap.ReportFXRate)
		assert.Equal(t, rateID, *snap.ReportFXRateID)
	})

	t.Run("missing rate returns nil snapshot without blocking charge", func(t *testing.T) {
		repo.On("Latest", ctx, "EUR", "INR").Return(nil, domain.ErrNoRate).Once()

		snap, err := snapshotter.Snapshot(ctx, "EUR")
		require.NoError(t, err)
		assert.Nil(t, snap)
	})

	t.Run("quarantined rate returns nil snapshot without blocking charge", func(t *testing.T) {
		rate := &domain.Rate{
			ID:          200,
			Base:        "USD",
			Quote:       "INR",
			RateDecimal: "150.00000000",
			AsOf:        now,
			Status:      domain.RateStatusQuarantined,
		}
		repo.On("Latest", ctx, "USD", "INR").Return(rate, nil).Once()

		snap, err := snapshotter.Snapshot(ctx, "USD")
		require.NoError(t, err)
		assert.Nil(t, snap)
	})

	t.Run("unusable rate (>7d) returns nil snapshot without blocking charge", func(t *testing.T) {
		rate := &domain.Rate{
			ID:          300,
			Base:        "USD",
			Quote:       "INR",
			RateDecimal: "83.12000000",
			AsOf:        now.Add(-10 * 24 * time.Hour),
			Status:      domain.RateStatusAccepted,
		}
		repo.On("Latest", ctx, "USD", "INR").Return(rate, nil).Once()

		snap, err := snapshotter.Snapshot(ctx, "USD")
		require.NoError(t, err)
		assert.Nil(t, snap)
	})

	repo.AssertExpectations(t)
}

func TestService_GetLatestRate_InverseFallback(t *testing.T) {
	repo := new(mockRateRepo)
	svc := application.NewService(repo, nil, application.Config{
		StaleAfter: 36 * time.Hour,
		MaxAge:     7 * 24 * time.Hour,
	})

	ctx := context.Background()
	now := time.Now().UTC()

	directRate := &domain.Rate{
		ID:          1,
		Base:        "USD",
		Quote:       "INR",
		RateDecimal: "80.00000000",
		Rate:        big.NewRat(80, 1),
		AsOf:        now.Add(-1 * time.Hour),
		FetchedAt:   now.Add(-1 * time.Hour),
		Status:      domain.RateStatusAccepted,
	}

	// Requesting INR -> USD when only USD -> INR is stored
	repo.On("Latest", ctx, "INR", "USD").Return(nil, domain.ErrNoRate).Once()
	repo.On("Latest", ctx, "USD", "INR").Return(directRate, nil).Once()

	rate, err := svc.GetLatestRate(ctx, "INR", "USD")
	require.NoError(t, err)
	assert.Equal(t, "INR", rate.Base)
	assert.Equal(t, "USD", rate.Quote)
	assert.Equal(t, "0.01250000", rate.RateDecimal) // 1 / 80 = 0.0125
	assert.False(t, rate.Stale)
	assert.False(t, rate.Unusable)

	// When both direct and inverse are missing
	repo.On("Latest", ctx, "EUR", "GBP").Return(nil, domain.ErrNoRate).Once()
	repo.On("Latest", ctx, "GBP", "EUR").Return(nil, domain.ErrNoRate).Once()

	_, err = svc.GetLatestRate(ctx, "EUR", "GBP")
	require.ErrorIs(t, err, domain.ErrNoRate)

	repo.AssertExpectations(t)
}
