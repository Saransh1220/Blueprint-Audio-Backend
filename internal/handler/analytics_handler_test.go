package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/dto"
	"github.com/saransh1220/blueprint-audio/internal/handler"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/saransh1220/blueprint-audio/internal/mocks"
	"github.com/saransh1220/blueprint-audio/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestAnalyticsHandler_TrackPlayAndToggleFavorite(t *testing.T) {
	as := new(mockAnalyticsService)
	specRepo := new(mockSpecRepo)
	fileSvc := new(mocks.MockFileService)
	h := handler.NewAnalyticsHandler(as, specRepo, fileSvc)
	specID := uuid.New()
	userID := uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/play", nil)
	req.SetPathValue("id", specID.String())
	as.On("TrackPlay", mock.Anything, specID).Return(nil).Once()
	w := httptest.NewRecorder()
	h.TrackPlay(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/favorite", nil)
	req.SetPathValue("id", specID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	as.On("ToggleFavorite", mock.Anything, userID, specID).Return(true, nil).Once()
	as.On("GetPublicAnalytics", mock.Anything, specID, (*uuid.UUID)(nil)).Return(&service.PublicAnalytics{FavoriteCount: 9}, nil).Once()
	w = httptest.NewRecorder()
	h.ToggleFavorite(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"total_count\":9")
}

func TestAnalyticsHandler_GetProducerAndOverview(t *testing.T) {
	as := new(mockAnalyticsService)
	specRepo := new(mockSpecRepo)
	fileSvc := new(mocks.MockFileService)
	h := handler.NewAnalyticsHandler(as, specRepo, fileSvc)
	userID := uuid.New()
	specID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/specs/"+specID.String()+"/analytics", nil)
	req.SetPathValue("id", specID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	as.On("GetProducerAnalytics", mock.Anything, specID, userID).Return(nil, errors.New("unauthorized")).Once()
	w := httptest.NewRecorder()
	h.GetProducerAnalytics(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/analytics/overview?days=7&sortBy=revenue", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	as.On("GetStatsOverview", mock.Anything, userID, 7, "revenue").Return(&dto.AnalyticsOverviewResponse{TotalPlays: 1}, nil).Once()
	w = httptest.NewRecorder()
	h.GetOverview(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAnalyticsHandler_GetTopSpecs(t *testing.T) {
	as := new(mockAnalyticsService)
	specRepo := new(mockSpecRepo)
	fileSvc := new(mocks.MockFileService)
	h := handler.NewAnalyticsHandler(as, specRepo, fileSvc)
	userID := uuid.New()

	t.Run("unauthorized without user in context", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/analytics/top-specs", nil)
		w := httptest.NewRecorder()

		h.GetTopSpecs(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var body map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &body)
		assert.NoError(t, err)
		assert.Equal(t, "Unauthorized", body["error"])
	})

	t.Run("service failure", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/analytics/top-specs?sortBy=downloads", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
		as.On("GetTopSpecs", mock.Anything, userID, 5, "downloads").Return(nil, errors.New("db")).Once()

		w := httptest.NewRecorder()
		h.GetTopSpecs(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var body map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &body)
		assert.NoError(t, err)
		assert.Equal(t, "Failed to fetch top specs", body["error"])
	})

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/analytics/top-specs?sortBy=plays", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
		as.On("GetTopSpecs", mock.Anything, userID, 5, "plays").
			Return([]dto.TopSpecStat{{SpecID: uuid.NewString(), Title: "A", Plays: 10, Downloads: 2, Revenue: 20.5}}, nil).
			Once()

		w := httptest.NewRecorder()
		h.GetTopSpecs(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "\"title\":\"A\"")
		assert.Contains(t, w.Body.String(), "\"downloads\":2")
	})
}

func TestAnalyticsHandler_DownloadFreeMp3(t *testing.T) {
	as := new(mockAnalyticsService)
	specRepo := new(mockSpecRepo)
	fileSvc := new(mocks.MockFileService)
	h := handler.NewAnalyticsHandler(as, specRepo, fileSvc)
	specID := uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/download-free", nil)
	req.SetPathValue("id", specID.String())
	specRepo.On("GetByID", mock.Anything, specID).Return(&domain.Spec{
		ID:             specID,
		Title:          "Track",
		PreviewUrl:     "http://storage/bucket/preview.mp3",
		FreeMp3Enabled: true,
	}, nil).Once()
	as.On("TrackFreeDownload", mock.Anything, specID).Return(nil).Once()
	fileSvc.On("GetKeyFromUrl", "http://storage/bucket/preview.mp3").Return("preview.mp3", nil).Once()
	fileSvc.On("GetPresignedDownloadURL", mock.Anything, "preview.mp3", "Track.mp3", mock.Anything).Return("signed-download", nil).Once()

	w := httptest.NewRecorder()
	h.DownloadFreeMp3(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "signed-download")
}

func TestAnalyticsHandler_ExtraBranches(t *testing.T) {
	as := new(mockAnalyticsService)
	specRepo := new(mockSpecRepo)
	fileSvc := new(mocks.MockFileService)
	h := handler.NewAnalyticsHandler(as, specRepo, fileSvc)
	specID := uuid.New()
	userID := uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/specs/bad/play", nil)
	req.SetPathValue("id", "bad")
	w := httptest.NewRecorder()
	h.TrackPlay(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/play", nil)
	req.SetPathValue("id", specID.String())
	as.On("TrackPlay", mock.Anything, specID).Return(errors.New("db")).Once()
	w = httptest.NewRecorder()
	h.TrackPlay(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/favorite", nil)
	req.SetPathValue("id", specID.String())
	w = httptest.NewRecorder()
	h.ToggleFavorite(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Producer analytics success
	req = httptest.NewRequest(http.MethodGet, "/specs/"+specID.String()+"/analytics", nil)
	req.SetPathValue("id", specID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	as.On("GetProducerAnalytics", mock.Anything, specID, userID).Return(&service.ProducerAnalytics{
		PublicAnalytics:    service.PublicAnalytics{PlayCount: 1, FavoriteCount: 2, TotalDownloadCount: 3},
		TotalPurchaseCount: 4,
		PurchasesByLicense: map[string]int{"Basic": 1},
		TotalRevenue:       20,
	}, nil).Once()
	w = httptest.NewRecorder()
	h.GetProducerAnalytics(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Download free disabled and fallback
	req = httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/download-free", nil)
	req.SetPathValue("id", specID.String())
	specRepo.On("GetByID", mock.Anything, specID).Return(&domain.Spec{ID: specID, FreeMp3Enabled: false}, nil).Once()
	w = httptest.NewRecorder()
	h.DownloadFreeMp3(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/download-free", nil)
	req.SetPathValue("id", specID.String())
	specRepo.On("GetByID", mock.Anything, specID).Return(&domain.Spec{ID: specID, Title: "T", PreviewUrl: "plain-url", FreeMp3Enabled: true}, nil).Once()
	as.On("TrackFreeDownload", mock.Anything, specID).Return(nil).Once()
	fileSvc.On("GetKeyFromUrl", "plain-url").Return("", errors.New("no-key")).Once()
	w = httptest.NewRecorder()
	h.DownloadFreeMp3(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "plain-url")
}
