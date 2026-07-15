package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	analyticsDomain "github.com/saransh1220/blueprint-audio/internal/modules/analytics/domain"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	catalogHTTP "github.com/saransh1220/blueprint-audio/internal/modules/catalog/interfaces/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newHandler() (*catalogHTTP.SpecHandler, *mockSpecService, *mockFileService, *mockAnalyticsService, *mockNotificationService) {
	specSvc := new(mockSpecService)
	fileSvc := new(mockFileService)
	analyticsSvc := new(mockAnalyticsService)
	notificationSvc := new(mockNotificationService)
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	h := catalogHTTP.NewSpecHandler(specSvc, fileSvc, analyticsSvc, notificationSvc, rdb)
	return h, specSvc, fileSvc, analyticsSvc, notificationSvc
}

func TestSpecHandler_BasicValidationBranches(t *testing.T) {
	h, specSvc, _, _, _ := newHandler()

	req := httptest.NewRequest(http.MethodPost, "/specs", bytes.NewBufferString("not-multipart"))
	w := httptest.NewRecorder()
	h.Create(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/specs/bad-id", nil)
	req.SetPathValue("id", "bad-id")
	w = httptest.NewRecorder()
	specSvc.On("GetSpecBySlug", mock.Anything, "bad-id").Return((*catalogDomain.Spec)(nil), nil).Once()
	h.Get(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/specs", nil)
	specSvc.On("ListSpecs", mock.Anything, mock.Anything).Return(nil, 0, assert.AnError).Once()
	w = httptest.NewRecorder()
	h.List(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	req = httptest.NewRequest(http.MethodDelete, "/specs/bad", nil)
	req.SetPathValue("id", "bad")
	w = httptest.NewRecorder()
	h.Delete(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	req = httptest.NewRequest(http.MethodPatch, "/specs/bad", nil)
	req.SetPathValue("id", "bad")
	w = httptest.NewRecorder()
	h.Update(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/users/bad/specs", nil)
	req.SetPathValue("id", "bad")
	w = httptest.NewRecorder()
	h.GetUserSpecs(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSpecHandler_ListAndGetUserSpecs_Success(t *testing.T) {
	h, specSvc, fileSvc, analyticsSvc, _ := newHandler()

	specID := uuid.New()
	userID := uuid.New()
	specs := []catalogDomain.Spec{
		{
			ID:         specID,
			ProducerID: userID,
			Title:      "Track",
			ImageUrl:   "http://storage/bucket/img.jpg",
			PreviewUrl: "http://storage/bucket/preview.mp3",
		},
	}

	specSvc.On("ListSpecs", mock.Anything, mock.Anything).Return(specs, 1, nil).Once()
	fileSvc.On("GetKeyFromUrl", "http://storage/bucket/preview.mp3").Return("preview.mp3", nil).Once()
	fileSvc.On("GetPresignedDownloadURL", mock.Anything, "preview.mp3", "Track", mock.Anything).Return("signed-preview", nil).Once()
	fileSvc.On("GetKeyFromUrl", "http://storage/bucket/img.jpg").Return("img.jpg", nil).Once()
	fileSvc.On("GetPresignedURL", mock.Anything, "img.jpg", mock.Anything).Return("signed-img", nil).Once()
	analyticsSvc.On("GetPublicAnalytics", mock.Anything, specID, (*uuid.UUID)(nil)).
		Return(&analyticsDomain.PublicAnalytics{PlayCount: 1, FavoriteCount: 2, IsFavorited: false}, nil).Once()

	req := httptest.NewRequest(http.MethodGet, "/specs", nil)
	w := httptest.NewRecorder()
	h.List(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	specSvc.On("GetUserSpecs", mock.Anything, userID, 1, 20).Return(specs, 1, nil).Once()
	fileSvc.On("GetKeyFromUrl", "signed-preview").Return("", assert.AnError).Once()
	fileSvc.On("GetKeyFromUrl", "signed-img").Return("", assert.AnError).Once()
	analyticsSvc.On("GetPublicAnalytics", mock.Anything, specID, (*uuid.UUID)(nil)).
		Return(&analyticsDomain.PublicAnalytics{PlayCount: 1, FavoriteCount: 2, IsFavorited: false}, nil).Once()

	req = httptest.NewRequest(http.MethodGet, "/users/"+userID.String()+"/specs?page=1", nil)
	req.SetPathValue("id", userID.String())
	w = httptest.NewRecorder()
	h.GetUserSpecs(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSpecHandler_Home_Success(t *testing.T) {
	h, specSvc, _, analyticsSvc, _ := newHandler()

	specID := uuid.New()
	viewerID := uuid.New()
	spec := catalogDomain.Spec{
		ID:               specID,
		ProducerID:       uuid.New(),
		ProducerName:     "Producer",
		Title:            "Chart Beat",
		Category:         catalogDomain.CategoryBeat,
		Type:             "beat",
		BPM:              140,
		Key:              "E MINOR",
		ProcessingStatus: catalogDomain.ProcessingStatusCompleted,
	}
	score := 42.0
	metrics := catalogDomain.BeatRankingMetrics{
		Plays:     10,
		Favorites: 2,
		Purchases: 1,
	}
	home := &catalogDomain.HomepageData{
		GeneratedAt:     spec.CreatedAt,
		CacheTTLSeconds: 900,
		Stats: catalogDomain.HomepageStats{
			TotalLiveBeats: 3,
			NewReleases7D:  1,
			TotalProducers: 2,
		},
		Sections: map[string]catalogDomain.HomepageSection{
			catalogDomain.HomeSectionTopCharts: {
				Title:  "Top charts",
				Source: "algorithmic",
				Period: catalogDomain.HomePeriod7D,
				Items: []catalogDomain.RankedSpec{
					{
						Spec:     spec,
						Rank:     1,
						Score:    &score,
						Movement: "up",
						Metrics:  &metrics,
					},
				},
			},
		},
	}

	specSvc.On("GetHome", mock.Anything, catalogDomain.HomepageParams{
		Limit:    6,
		Sections: []string{"top_charts"},
		Period:   "7d",
	}).Return(home, nil).Once()
	analyticsSvc.On("GetPublicAnalytics", mock.Anything, specID, &viewerID).
		Return(&analyticsDomain.PublicAnalytics{PlayCount: 10, FavoriteCount: 2, IsFavorited: true}, nil).Once()

	req := httptest.NewRequest(http.MethodGet, "/catalog/home?limit=6&period=7d&sections=top_charts", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, viewerID))
	w := httptest.NewRecorder()

	h.Home(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response catalogHTTP.HomepageResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Equal(t, 3, response.Stats.TotalLiveBeats)
	item := response.Sections["top_charts"].Items[0]
	assert.Equal(t, "Chart Beat", item.Spec.Title)
	assert.Equal(t, "up", item.Movement)
	assert.True(t, item.Spec.Analytics.IsFavorited)
}

func TestSpecHandler_GetAndDeleteFlow(t *testing.T) {
	h, specSvc, fileSvc, analyticsSvc, _ := newHandler()

	specID := uuid.New()
	producerID := uuid.New()
	spec := &catalogDomain.Spec{
		ID:         specID,
		ProducerID: producerID,
		Title:      "Track",
		ImageUrl:   "http://storage/bucket/img.jpg",
		PreviewUrl: "http://storage/bucket/preview.mp3",
	}

	specSvc.On("GetSpec", mock.Anything, specID).Return(spec, nil).Once()
	fileSvc.On("GetKeyFromUrl", "http://storage/bucket/preview.mp3").Return("preview.mp3", nil).Once()
	fileSvc.On("GetPresignedDownloadURL", mock.Anything, "preview.mp3", "Track", mock.Anything).Return("signed-preview", nil).Once()
	fileSvc.On("GetKeyFromUrl", "http://storage/bucket/img.jpg").Return("img.jpg", nil).Once()
	fileSvc.On("GetPresignedURL", mock.Anything, "img.jpg", mock.Anything).Return("signed-img", nil).Once()
	analyticsSvc.On("GetPublicAnalytics", mock.Anything, specID, (*uuid.UUID)(nil)).
		Return(&analyticsDomain.PublicAnalytics{PlayCount: 1, FavoriteCount: 1}, nil).Once()

	req := httptest.NewRequest(http.MethodGet, "/specs/"+specID.String(), nil)
	req.SetPathValue("id", specID.String())
	w := httptest.NewRecorder()
	h.Get(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	specSvc.On("GetSpec", mock.Anything, specID).Return(spec, nil).Once()
	specSvc.On("DeleteSpec", mock.Anything, specID, producerID).Return(nil).Once()
	fileSvc.On("GetKeyFromUrl", "signed-img").Return("img.jpg", nil).Once()
	fileSvc.On("Delete", mock.Anything, "img.jpg").Return(nil).Once()
	fileSvc.On("GetKeyFromUrl", "signed-preview").Return("preview.mp3", nil).Once()
	fileSvc.On("Delete", mock.Anything, "preview.mp3").Return(nil).Once()

	req = httptest.NewRequest(http.MethodDelete, "/specs/"+specID.String(), nil)
	req.SetPathValue("id", specID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
	w = httptest.NewRecorder()
	h.Delete(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestSpecHandler_UpdateBranches(t *testing.T) {
	h, specSvc, _, _, _ := newHandler()

	specID := uuid.New()
	producerID := uuid.New()
	otherID := uuid.New()

	req := httptest.NewRequest(http.MethodPatch, "/specs/"+specID.String(), nil)
	req.SetPathValue("id", specID.String())
	w := httptest.NewRecorder()
	h.Update(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	specSvc.On("GetSpec", mock.Anything, specID).Return(&catalogDomain.Spec{ID: specID, ProducerID: otherID}, nil).Once()
	req = httptest.NewRequest(http.MethodPatch, "/specs/"+specID.String(), nil)
	req.SetPathValue("id", specID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
	w = httptest.NewRecorder()
	h.Update(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSpecHandler_UpdateSuccessWithoutImage(t *testing.T) {
	h, specSvc, fileSvc, analyticsSvc, notificationSvc := newHandler()
	defer specSvc.AssertExpectations(t)
	defer fileSvc.AssertExpectations(t)
	defer analyticsSvc.AssertExpectations(t)
	defer notificationSvc.AssertExpectations(t)
	id, producerID := uuid.New(), uuid.New()
	existing := &catalogDomain.Spec{ID: id, ProducerID: producerID, Title: "Old", Category: catalogDomain.CategorySample}
	specSvc.On("GetSpec", mock.Anything, id).Return(existing, nil).Once()
	specSvc.On("UpdateSpec", mock.Anything, mock.AnythingOfType("*domain.Spec"), producerID).Return(nil).Once()
	var body bytes.Buffer
	body.WriteString("--x\r\nContent-Disposition: form-data; name=\"metadata\"\r\n\r\n")
	body.WriteString(`{"title":"New","base_price":12,"bpm":90,"key":"C","free_mp3_enabled":true}`)
	body.WriteString("\r\n--x--\r\n")
	req := httptest.NewRequest(http.MethodPatch, "/specs/"+id.String(), &body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	req.SetPathValue("id", id.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
	w := httptest.NewRecorder()
	h.Update(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "New")
}

func TestSpecHandler_CreateBranches(t *testing.T) {
	h, specSvc, _, _, _ := newHandler()
	producerID := uuid.New()

	makeReq := func(metadata map[string]any) *http.Request {
		var b bytes.Buffer
		raw, _ := json.Marshal(metadata)
		b.WriteString("--x\r\nContent-Disposition: form-data; name=\"metadata\"\r\n\r\n")
		b.Write(raw)
		b.WriteString("\r\n--x--\r\n")
		req := httptest.NewRequest(http.MethodPost, "/specs", &b)
		req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
		return req
	}

	req := makeReq(map[string]any{"title": "T", "category": "sample", "price": 10})
	w := httptest.NewRecorder()
	h.Create(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	req = makeReq(map[string]any{"title": "T", "category": "sample", "price": 10})
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
	specSvc.On("CreateSpec", mock.Anything, mock.AnythingOfType("*domain.Spec")).Return(assert.AnError).Once()
	w = httptest.NewRecorder()
	h.Create(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Beat without wav/stems should fail validation before service call.
	req = makeReq(map[string]any{"title": "Beat", "category": "beat", "price": 10})
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
	w = httptest.NewRecorder()
	h.Create(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
