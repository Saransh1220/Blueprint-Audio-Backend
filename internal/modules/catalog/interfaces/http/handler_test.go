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
	h.Get(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

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

	specSvc.On("GetUserSpecs", mock.Anything, userID, 1).Return(specs, 1, nil).Once()
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
}
