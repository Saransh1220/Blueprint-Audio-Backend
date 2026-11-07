package http_test

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/fx/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/fx/domain"
	fxHttp "github.com/saransh1220/blueprint-audio/internal/modules/fx/interfaces/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockAuditLogger struct {
	mock.Mock
}

func (m *mockAuditLogger) Log(ctx context.Context, actorID *uuid.UUID, action, resourceType, resourceID string, beforeState, afterState any, ip, userAgent string) error {
	args := m.Called(ctx, actorID, action, resourceType, resourceID, beforeState, afterState, ip, userAgent)
	return args.Error(0)
}

type mockRateRepo struct {
	mock.Mock
}

func (m *mockRateRepo) Append(ctx context.Context, rate *domain.Rate) error {
	return m.Called(ctx, rate).Error(0)
}

func (m *mockRateRepo) AppendBatch(ctx context.Context, rates []*domain.Rate) error {
	return m.Called(ctx, rates).Error(0)
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

func TestFXHandler_AdminListRates(t *testing.T) {
	repo := new(mockRateRepo)
	svc := application.NewService(repo, nil, application.Config{})
	handler := fxHttp.NewFXHandler(svc, nil)

	now := time.Now().UTC()
	history := []*domain.Rate{
		{Base: "USD", Quote: "INR", RateDecimal: "83.12000000", AsOf: now, FetchedAt: now},
	}
	latest := []*domain.Rate{
		{Base: "USD", Quote: "INR", RateDecimal: "83.12000000", AsOf: now, FetchedAt: now},
	}

	repo.On("History", mock.Anything, mock.MatchedBy(func(f domain.HistoryFilter) bool {
		return f.Limit == 20 && f.Offset == 0
	})).Return(history, 1, nil).Once()

	repo.On("LatestPerPair", mock.Anything).Return(latest, nil).Once()

	req := httptest.NewRequest(http.MethodGet, "/admin/fx-rates", nil)
	w := httptest.NewRecorder()
	handler.AdminListRates(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, float64(1), body["metadata"].(map[string]any)["total"])
	assert.Len(t, body["data"], 1)
	assert.Len(t, body["latest"], 1)
}

func TestFXHandler_AdminRefreshRates(t *testing.T) {
	repo := new(mockRateRepo)
	lockMgr := new(mockLockManager)
	prov := new(mockRateProvider)
	cfg := application.Config{RefreshInterval: 24 * time.Hour}
	refresher := application.NewRefresher(repo, lockMgr, prov, cfg)
	svc := application.NewService(repo, refresher, cfg)
	audit := new(mockAuditLogger)
	handler := fxHttp.NewFXHandler(svc, audit)

	ctx := context.Background()
	now := time.Now().UTC()
	actorID := uuid.New()

	unlock := func() error { return nil }
	lockMgr.On("TryLock", mock.Anything, "fx_refresh").Return(unlock, true, nil).Once()

	prov.On("Fetch", mock.Anything, "INR", []string{"USD"}).Return(&domain.ProviderRates{
		Base: "INR", AsOf: now, Rates: map[string]*big.Rat{"USD": big.NewRat(12, 1000)}, Source: "mock",
	}, nil).Once()
	prov.On("Fetch", mock.Anything, "USD", []string{"INR"}).Return(&domain.ProviderRates{
		Base: "USD", AsOf: now, Rates: map[string]*big.Rat{"INR": big.NewRat(8312, 100)}, Source: "mock",
	}, nil).Once()

	repo.On("Latest", mock.Anything, "INR", "USD").Return(nil, domain.ErrNoRate).Once()
	repo.On("Latest", mock.Anything, "USD", "INR").Return(nil, domain.ErrNoRate).Once()
	repo.On("AppendBatch", mock.Anything, mock.Anything).Return(nil).Once()

	audit.On("Log", mock.Anything, &actorID, "fx:refresh", "fx_rates", "", nil, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Once()

	req := httptest.NewRequest(http.MethodPost, "/admin/fx-rates/refresh", nil)
	req = req.WithContext(context.WithValue(ctx, middleware.ContextKeyUserId, actorID))
	w := httptest.NewRecorder()
	handler.AdminRefreshRates(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "success", body["status"])
	assert.Equal(t, float64(2), body["rates_updated"])

	// Immediate second refresh triggers 429 cooldown
	w = httptest.NewRecorder()
	handler.AdminRefreshRates(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestFXHandler_GetLatestRate(t *testing.T) {
	repo := new(mockRateRepo)
	svc := application.NewService(repo, nil, application.Config{MaxAge: 7 * 24 * time.Hour})
	handler := fxHttp.NewFXHandler(svc, nil)

	now := time.Now().UTC()

	t.Run("missing params -> 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/fx/rates/latest", nil)
		w := httptest.NewRecorder()
		handler.GetLatestRate(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("unsupported currency -> 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/fx/rates/latest?base=XYZ&quote=INR", nil)
		w := httptest.NewRecorder()
		handler.GetLatestRate(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("same currency identity -> 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/fx/rates/latest?base=USD&quote=USD", nil)
		w := httptest.NewRecorder()
		handler.GetLatestRate(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"rate":"1.00000000"`)
	})

	t.Run("rate not found -> 404", func(t *testing.T) {
		repo.On("Latest", mock.Anything, "USD", "INR").Return(nil, domain.ErrNoRate).Once()
		req := httptest.NewRequest(http.MethodGet, "/fx/rates/latest?base=USD&quote=INR", nil)
		w := httptest.NewRecorder()
		handler.GetLatestRate(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("rate unusable -> 409", func(t *testing.T) {
		oldRate := &domain.Rate{
			Base:        "USD",
			Quote:       "INR",
			RateDecimal: "83.12000000",
			AsOf:        now.Add(-10 * 24 * time.Hour), // 10 days old > 7 days
			Status:      domain.RateStatusAccepted,
		}
		repo.On("Latest", mock.Anything, "USD", "INR").Return(oldRate, nil).Once()
		req := httptest.NewRequest(http.MethodGet, "/fx/rates/latest?base=USD&quote=INR", nil)
		w := httptest.NewRecorder()
		handler.GetLatestRate(w, req)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("valid rate -> 200", func(t *testing.T) {
		validRate := &domain.Rate{
			Base:        "USD",
			Quote:       "INR",
			RateDecimal: "83.12000000",
			AsOf:        now.Add(-2 * time.Hour),
			FetchedAt:   now.Add(-1 * time.Hour),
			Source:      "frankfurter",
			Status:      domain.RateStatusAccepted,
		}
		repo.On("Latest", mock.Anything, "USD", "INR").Return(validRate, nil).Once()
		req := httptest.NewRequest(http.MethodGet, "/fx/rates/latest?base=USD&quote=INR", nil)
		w := httptest.NewRecorder()
		handler.GetLatestRate(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"rate":"83.12000000"`)
		assert.Contains(t, w.Body.String(), `"source":"frankfurter"`)
	})
}
