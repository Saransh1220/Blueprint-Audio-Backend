package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	cachedb "github.com/saransh1220/blueprint-audio/internal/db"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/handler"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/saransh1220/blueprint-audio/internal/mocks"
	"github.com/saransh1220/blueprint-audio/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSpecHandler_CreateGetListDeleteUpdateGetUserSpecs(t *testing.T) {
	specSvc := new(mockSpecService)
	analyticsSvc := new(mockAnalyticsService)
	fileSvc := new(mocks.MockFileService)
	h := handler.NewSpecHandler(specSvc, fileSvc, analyticsSvc)

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

	userID := uuid.New()
	req = httptest.NewRequest(http.MethodDelete, "/specs/"+uuid.New().String(), nil)
	req.SetPathValue("id", uuid.New().String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	specSvc.On("GetSpec", mock.Anything, mock.Anything).Return(nil, assert.AnError).Once()
	w = httptest.NewRecorder()
	h.Delete(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSpecHandler_ListAndGetUserSpecsSuccess(t *testing.T) {
	specSvc := new(mockSpecService)
	analyticsSvc := new(mockAnalyticsService)
	fileSvc := new(mocks.MockFileService)
	h := handler.NewSpecHandler(specSvc, fileSvc, analyticsSvc)

	specID := uuid.New()
	userID := uuid.New()
	specs := []domain.Spec{
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
	fileSvc.On("GetPresignedURL", mock.Anything, "preview.mp3", mock.Anything).Return("signed-preview", nil).Once()
	fileSvc.On("GetKeyFromUrl", "http://storage/bucket/img.jpg").Return("img.jpg", nil).Once()
	fileSvc.On("GetPresignedURL", mock.Anything, "img.jpg", mock.Anything).Return("signed-img", nil).Once()
	analyticsSvc.On("GetPublicAnalytics", mock.Anything, specID, (*uuid.UUID)(nil)).
		Return(&service.PublicAnalytics{PlayCount: 1, FavoriteCount: 2, IsFavorited: false}, nil).Once()

	req := httptest.NewRequest(http.MethodGet, "/specs", nil)
	w := httptest.NewRecorder()
	h.List(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	specSvc.On("GetUserSpecs", mock.Anything, userID, 1).Return(specs, 1, nil).Once()
	fileSvc.On("GetKeyFromUrl", "signed-preview").Return("", assert.AnError).Once()
	fileSvc.On("GetKeyFromUrl", "signed-img").Return("", assert.AnError).Once()
	analyticsSvc.On("GetPublicAnalytics", mock.Anything, specID, (*uuid.UUID)(nil)).
		Return(&service.PublicAnalytics{PlayCount: 1, FavoriteCount: 2, IsFavorited: false}, nil).Once()

	req = httptest.NewRequest(http.MethodGet, "/users/"+userID.String()+"/specs?page=1", nil)
	req.SetPathValue("id", userID.String())
	w = httptest.NewRecorder()
	h.GetUserSpecs(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSpecHandler_GetAndDeleteWithCacheClient(t *testing.T) {
	cachedb.Rdb = redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})

	specSvc := new(mockSpecService)
	analyticsSvc := new(mockAnalyticsService)
	fileSvc := new(mocks.MockFileService)
	h := handler.NewSpecHandler(specSvc, fileSvc, analyticsSvc)

	specID := uuid.New()
	producerID := uuid.New()
	spec := &domain.Spec{
		ID:         specID,
		ProducerID: producerID,
		Title:      "Track",
		ImageUrl:   "http://storage/bucket/img.jpg",
		PreviewUrl: "http://storage/bucket/preview.mp3",
	}

	specSvc.On("GetSpec", mock.Anything, specID).Return(spec, nil).Once()
	fileSvc.On("GetKeyFromUrl", "http://storage/bucket/preview.mp3").Return("preview.mp3", nil).Once()
	fileSvc.On("GetPresignedURL", mock.Anything, "preview.mp3", mock.Anything).Return("signed-preview", nil).Once()
	fileSvc.On("GetKeyFromUrl", "http://storage/bucket/img.jpg").Return("img.jpg", nil).Once()
	fileSvc.On("GetPresignedURL", mock.Anything, "img.jpg", mock.Anything).Return("signed-img", nil).Once()
	analyticsSvc.On("GetPublicAnalytics", mock.Anything, specID, (*uuid.UUID)(nil)).
		Return(&service.PublicAnalytics{PlayCount: 1, FavoriteCount: 1}, nil).Once()

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
	cachedb.Rdb = redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})

	specSvc := new(mockSpecService)
	analyticsSvc := new(mockAnalyticsService)
	fileSvc := new(mocks.MockFileService)
	h := handler.NewSpecHandler(specSvc, fileSvc, analyticsSvc)

	specID := uuid.New()
	producerID := uuid.New()
	otherID := uuid.New()

	// Unauthorized
	req := httptest.NewRequest(http.MethodPatch, "/specs/"+specID.String(), nil)
	req.SetPathValue("id", specID.String())
	w := httptest.NewRecorder()
	h.Update(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Not owner
	specSvc.On("GetSpec", mock.Anything, specID).Return(&domain.Spec{ID: specID, ProducerID: otherID}, nil).Once()
	req = httptest.NewRequest(http.MethodPatch, "/specs/"+specID.String(), nil)
	req.SetPathValue("id", specID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
	w = httptest.NewRecorder()
	h.Update(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSpecHandler_GetCacheHit(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	cachedb.Rdb = redis.NewClient(&redis.Options{Addr: mr.Addr()})

	specID := uuid.New()
	require.NoError(t, mr.Set("spec:"+specID.String(), `{"id":"`+specID.String()+`","title":"cached"}`))

	specSvc := new(mockSpecService)
	analyticsSvc := new(mockAnalyticsService)
	fileSvc := new(mocks.MockFileService)
	h := handler.NewSpecHandler(specSvc, fileSvc, analyticsSvc)

	req := httptest.NewRequest(http.MethodGet, "/specs/"+specID.String(), nil)
	req.SetPathValue("id", specID.String())
	w := httptest.NewRecorder()
	h.Get(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "HIT", w.Header().Get("X-Cache"))
	assert.Contains(t, w.Body.String(), "cached")
}

func TestSpecHandler_CreateAndDeleteExtraBranches(t *testing.T) {
	specSvc := new(mockSpecService)
	analyticsSvc := new(mockAnalyticsService)
	fileSvc := new(mocks.MockFileService)
	h := handler.NewSpecHandler(specSvc, fileSvc, analyticsSvc)
	producerID := uuid.New()

	makeReq := func(metadata map[string]any) *http.Request {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		raw, _ := json.Marshal(metadata)
		_ = mw.WriteField("metadata", string(raw))
		_ = mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/specs", &b)
		req.Header.Set("Content-Type", mw.FormDataContentType())
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

	req = makeReq(map[string]any{"title": "T", "category": "sample", "price": 10})
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
	specSvc.On("CreateSpec", mock.Anything, mock.AnythingOfType("*domain.Spec")).Return(nil).Once()
	w = httptest.NewRecorder()
	h.Create(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	specID := uuid.New()
	specSvc.On("GetSpec", mock.Anything, specID).Return(&domain.Spec{ID: specID, ProducerID: producerID}, nil).Once()
	specSvc.On("DeleteSpec", mock.Anything, specID, producerID).Return(domain.ErrSpecSoftDeleted).Once()
	req = httptest.NewRequest(http.MethodDelete, "/specs/"+specID.String(), nil)
	req.SetPathValue("id", specID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
	w = httptest.NewRecorder()
	h.Delete(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestSpecHandler_UpdateSuccessAndBadMetadata(t *testing.T) {
	cachedb.Rdb = redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})

	specSvc := new(mockSpecService)
	analyticsSvc := new(mockAnalyticsService)
	fileSvc := new(mocks.MockFileService)
	h := handler.NewSpecHandler(specSvc, fileSvc, analyticsSvc)

	specID := uuid.New()
	producerID := uuid.New()
	existing := &domain.Spec{
		ID:         specID,
		ProducerID: producerID,
		Title:      "Old",
		ImageUrl:   "http://storage/bucket/img.jpg",
		PreviewUrl: "http://storage/bucket/preview.mp3",
	}

	// Success path with valid multipart metadata and no image file
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	raw, _ := json.Marshal(map[string]any{
		"title":            "New",
		"price":            10,
		"bpm":              120,
		"key":              "C",
		"description":      "desc",
		"free_mp3_enabled": true,
	})
	_ = mw.WriteField("metadata", string(raw))
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPatch, "/specs/"+specID.String(), &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("id", specID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))

	specSvc.On("GetSpec", mock.Anything, specID).Return(existing, nil).Once()
	specSvc.On("UpdateSpec", mock.Anything, mock.AnythingOfType("*domain.Spec"), producerID).Return(nil).Once()
	fileSvc.On("GetKeyFromUrl", "http://storage/bucket/preview.mp3").Return("preview.mp3", nil).Once()
	fileSvc.On("GetPresignedURL", mock.Anything, "preview.mp3", mock.Anything).Return("signed-preview", nil).Once()
	fileSvc.On("GetKeyFromUrl", "http://storage/bucket/img.jpg").Return("img.jpg", nil).Once()
	fileSvc.On("GetPresignedURL", mock.Anything, "img.jpg", mock.Anything).Return("signed-img", nil).Once()

	w := httptest.NewRecorder()
	h.Update(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "signed-preview")

	// Invalid metadata path
	b.Reset()
	mw = multipart.NewWriter(&b)
	_ = mw.WriteField("metadata", "{invalid")
	_ = mw.Close()
	req = httptest.NewRequest(http.MethodPatch, "/specs/"+specID.String(), &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("id", specID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
	specSvc.On("GetSpec", mock.Anything, specID).Return(&domain.Spec{ID: specID, ProducerID: producerID}, nil).Once()
	w = httptest.NewRecorder()
	h.Update(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSpecHandler_DeleteForbiddenAndNotFound(t *testing.T) {
	specSvc := new(mockSpecService)
	analyticsSvc := new(mockAnalyticsService)
	fileSvc := new(mocks.MockFileService)
	h := handler.NewSpecHandler(specSvc, fileSvc, analyticsSvc)

	specID := uuid.New()
	producerID := uuid.New()
	otherID := uuid.New()

	req := httptest.NewRequest(http.MethodDelete, "/specs/"+specID.String(), nil)
	req.SetPathValue("id", specID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
	specSvc.On("GetSpec", mock.Anything, specID).Return((*domain.Spec)(nil), nil).Once()
	w := httptest.NewRecorder()
	h.Delete(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	req = httptest.NewRequest(http.MethodDelete, "/specs/"+specID.String(), nil)
	req.SetPathValue("id", specID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
	specSvc.On("GetSpec", mock.Anything, specID).Return(&domain.Spec{ID: specID, ProducerID: otherID}, nil).Once()
	w = httptest.NewRecorder()
	h.Delete(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
