package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	analyticsApp "github.com/saransh1220/blueprint-audio/internal/modules/analytics/application"
	analyticsDomain "github.com/saransh1220/blueprint-audio/internal/modules/analytics/domain"
	analyticsHTTP "github.com/saransh1220/blueprint-audio/internal/modules/analytics/interfaces/http"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockAnalyticsService struct{ mock.Mock }

func (m *mockAnalyticsService) TrackPlay(ctx context.Context, specID uuid.UUID) error {
	args := m.Called(ctx, specID)
	return args.Error(0)
}
func (m *mockAnalyticsService) TrackFreeDownload(ctx context.Context, specID uuid.UUID) error {
	args := m.Called(ctx, specID)
	return args.Error(0)
}
func (m *mockAnalyticsService) ToggleFavorite(ctx context.Context, userID, specID uuid.UUID) (bool, error) {
	args := m.Called(ctx, userID, specID)
	return args.Bool(0), args.Error(1)
}
func (m *mockAnalyticsService) IsFavorited(ctx context.Context, userID, specID uuid.UUID) (bool, error) {
	args := m.Called(ctx, userID, specID)
	return args.Bool(0), args.Error(1)
}
func (m *mockAnalyticsService) GetPublicAnalytics(ctx context.Context, specID uuid.UUID, userID *uuid.UUID) (*analyticsDomain.PublicAnalytics, error) {
	args := m.Called(ctx, specID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*analyticsDomain.PublicAnalytics), args.Error(1)
}
func (m *mockAnalyticsService) GetProducerAnalytics(ctx context.Context, specID, producerID uuid.UUID) (*analyticsDomain.ProducerAnalytics, error) {
	args := m.Called(ctx, specID, producerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*analyticsDomain.ProducerAnalytics), args.Error(1)
}
func (m *mockAnalyticsService) GetStatsOverview(ctx context.Context, producerID uuid.UUID, days int, sortBy string) (*analyticsDomain.AnalyticsOverviewResponse, error) {
	args := m.Called(ctx, producerID, days, sortBy)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*analyticsDomain.AnalyticsOverviewResponse), args.Error(1)
}
func (m *mockAnalyticsService) GetTopSpecs(ctx context.Context, producerID uuid.UUID, limit int, sortBy string) ([]analyticsDomain.TopSpecStat, error) {
	args := m.Called(ctx, producerID, limit, sortBy)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]analyticsDomain.TopSpecStat), args.Error(1)
}

type mockSpecRepo struct{ mock.Mock }

func (m *mockSpecRepo) Create(ctx context.Context, spec *catalogDomain.Spec) error { return nil }
func (m *mockSpecRepo) GetByID(ctx context.Context, id uuid.UUID) (*catalogDomain.Spec, error) {
	return nil, nil
}
func (m *mockSpecRepo) GetByIDSystem(ctx context.Context, id uuid.UUID) (*catalogDomain.Spec, error) {
	return nil, nil
}
func (m *mockSpecRepo) List(ctx context.Context, filter catalogDomain.SpecFilter) ([]catalogDomain.Spec, int, error) {
	return nil, 0, nil
}
func (m *mockSpecRepo) Update(ctx context.Context, spec *catalogDomain.Spec) error { return nil }
func (m *mockSpecRepo) Delete(ctx context.Context, id uuid.UUID, producerID uuid.UUID) error {
	return nil
}
func (m *mockSpecRepo) ListByUserID(ctx context.Context, producerID uuid.UUID, limit, offset int) ([]catalogDomain.Spec, int, error) {
	return nil, 0, nil
}

type mockFileService struct{ mock.Mock }

func (m *mockFileService) GetPresignedDownloadURL(ctx context.Context, key string, filename string, expiration time.Duration) (string, error) {
	args := m.Called(ctx, key, filename, expiration)
	return args.String(0), args.Error(1)
}
func (m *mockFileService) GetKeyFromUrl(url string) (string, error) {
	args := m.Called(url)
	return args.String(0), args.Error(1)
}

func TestAnalyticsHandler_Branches(t *testing.T) {
	as := new(mockAnalyticsService)
	specRepo := new(mockSpecRepo)
	fileSvc := new(mockFileService)
	var _ analyticsApp.AnalyticsService = as
	h := analyticsHTTP.NewAnalyticsHandler(as, specRepo, fileSvc)
	specID := uuid.New()
	userID := uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/specs/bad/play", nil)
	req.SetPathValue("id", "bad")
	w := httptest.NewRecorder()
	h.TrackPlay(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/play", nil)
	req.SetPathValue("id", specID.String())
	as.On("TrackPlay", mock.Anything, specID).Return(nil).Once()
	w = httptest.NewRecorder()
	h.TrackPlay(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/favorite", nil)
	req.SetPathValue("id", specID.String())
	w = httptest.NewRecorder()
	h.ToggleFavorite(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/favorite", nil)
	req.SetPathValue("id", specID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	as.On("ToggleFavorite", mock.Anything, userID, specID).Return(true, nil).Once()
	w = httptest.NewRecorder()
	h.ToggleFavorite(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "is_favorited")
}

func TestAnalyticsHandler_OverviewAndTopSpecs(t *testing.T) {
	as := new(mockAnalyticsService)
	specRepo := new(mockSpecRepo)
	fileSvc := new(mockFileService)
	h := analyticsHTTP.NewAnalyticsHandler(as, specRepo, fileSvc)
	userID := uuid.New()
	specID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/specs/"+specID.String()+"/analytics", nil)
	req.SetPathValue("id", specID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	as.On("GetProducerAnalytics", mock.Anything, specID, userID).Return(&analyticsDomain.ProducerAnalytics{}, nil).Once()
	w := httptest.NewRecorder()
	h.GetProducerAnalytics(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/analytics/overview?days=7&sort=revenue", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	as.On("GetStatsOverview", mock.Anything, userID, 7, "revenue").Return(&analyticsDomain.AnalyticsOverviewResponse{}, nil).Once()
	w = httptest.NewRecorder()
	h.GetOverview(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/analytics/top-specs?sortBy=plays", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	as.On("GetTopSpecs", mock.Anything, userID, 5, "plays").Return([]analyticsDomain.TopSpecStat{{SpecID: uuid.New(), Title: "A"}}, nil).Once()
	w = httptest.NewRecorder()
	h.GetTopSpecs(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAnalyticsHandler_GetProducerAnalytics_ErrorMapping(t *testing.T) {
	as := new(mockAnalyticsService)
	specRepo := new(mockSpecRepo)
	fileSvc := new(mockFileService)
	h := analyticsHTTP.NewAnalyticsHandler(as, specRepo, fileSvc)
	userID := uuid.New()
	specID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/specs/"+specID.String()+"/analytics", nil)
	req.SetPathValue("id", specID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	as.On("GetProducerAnalytics", mock.Anything, specID, userID).Return(nil, errors.New("unauthorized")).Once()
	w := httptest.NewRecorder()
	h.GetProducerAnalytics(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/specs/"+specID.String()+"/analytics", nil)
	req.SetPathValue("id", specID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	as.On("GetProducerAnalytics", mock.Anything, specID, userID).Return(nil, errors.New("spec not found")).Once()
	w = httptest.NewRecorder()
	h.GetProducerAnalytics(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
