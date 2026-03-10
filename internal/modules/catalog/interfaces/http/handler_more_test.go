package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func makeMultipartRequest(t *testing.T, method, path string, metadata map[string]interface{}) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	metaRaw, err := json.Marshal(metadata)
	assert.NoError(t, err)
	assert.NoError(t, writer.WriteField("metadata", string(metaRaw)))
	assert.NoError(t, writer.Close())

	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func makeMultipartRequestWithFile(t *testing.T, method, path string, metadata map[string]interface{}, field, filename string, content []byte) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	metaRaw, err := json.Marshal(metadata)
	assert.NoError(t, err)
	assert.NoError(t, writer.WriteField("metadata", string(metaRaw)))
	part, err := writer.CreateFormFile(field, filename)
	assert.NoError(t, err)
	_, err = io.Copy(part, bytes.NewReader(content))
	assert.NoError(t, err)
	assert.NoError(t, writer.Close())

	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestSpecHandler_DownloadFree_Branches(t *testing.T) {
	h, specSvc, fileSvc, analyticsSvc, _ := newHandler()
	specID := uuid.New()
	analyticsSvc.On("TrackFreeDownload", mock.Anything, specID).Return(nil).Maybe()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/specs/bad/download-free", nil)
	req.SetPathValue("id", "bad")
	h.DownloadFree(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/download-free", nil)
	req.SetPathValue("id", specID.String())
	specSvc.On("GetSpec", mock.Anything, specID).Return(nil, errors.New("db")).Once()
	h.DownloadFree(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/download-free", nil)
	req.SetPathValue("id", specID.String())
	specSvc.On("GetSpec", mock.Anything, specID).Return((*domain.Spec)(nil), nil).Once()
	h.DownloadFree(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	s := &domain.Spec{ID: specID, Title: "T", FreeMp3Enabled: false}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/download-free", nil)
	req.SetPathValue("id", specID.String())
	specSvc.On("GetSpec", mock.Anything, specID).Return(s, nil).Once()
	h.DownloadFree(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	s = &domain.Spec{ID: specID, Title: "T", FreeMp3Enabled: true}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/download-free", nil)
	req.SetPathValue("id", specID.String())
	specSvc.On("GetSpec", mock.Anything, specID).Return(s, nil).Once()
	h.DownloadFree(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	s = &domain.Spec{ID: specID, Title: "T", FreeMp3Enabled: true, PreviewUrl: "http://bucket/p.mp3"}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/download-free", nil)
	req.SetPathValue("id", specID.String())
	specSvc.On("GetSpec", mock.Anything, specID).Return(s, nil).Once()
	fileSvc.On("GetKeyFromUrl", s.PreviewUrl).Return("", errors.New("bad")).Once()
	h.DownloadFree(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/download-free", nil)
	req.SetPathValue("id", specID.String())
	specSvc.On("GetSpec", mock.Anything, specID).Return(s, nil).Once()
	fileSvc.On("GetKeyFromUrl", s.PreviewUrl).Return("p.mp3", nil).Once()
	fileSvc.On("GetPresignedDownloadURL", mock.Anything, "p.mp3", "T.mp3", mock.Anything).Return("", errors.New("x")).Once()
	h.DownloadFree(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/specs/"+specID.String()+"/download-free", nil)
	req.SetPathValue("id", specID.String())
	specSvc.On("GetSpec", mock.Anything, specID).Return(s, nil).Once()
	fileSvc.On("GetKeyFromUrl", s.PreviewUrl).Return("p.mp3", nil).Once()
	fileSvc.On("GetPresignedDownloadURL", mock.Anything, "p.mp3", "T.mp3", mock.Anything).Return("signed", nil).Once()
	h.DownloadFree(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSpecHandler_GetAndDelete_ErrorBranches(t *testing.T) {
	h, specSvc, _, _, _ := newHandler()
	specID := uuid.New()
	producerID := uuid.New()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/specs/"+specID.String(), nil)
	req.SetPathValue("id", specID.String())
	specSvc.On("GetSpec", mock.Anything, specID).Return(nil, errors.New("db")).Once()
	h.Get(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/specs/"+specID.String(), nil)
	req.SetPathValue("id", specID.String())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
	specSvc.On("GetSpec", mock.Anything, specID).Return(&domain.Spec{ID: specID, ProducerID: producerID}, nil).Once()
	specSvc.On("DeleteSpec", mock.Anything, specID, producerID).Return(domain.ErrSpecNotFound).Once()
	h.Delete(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSpecHandler_List_Get_Update_Delete_GetUserSpecs_Branches(t *testing.T) {
	t.Run("list builds filters and supports authenticated analytics", func(t *testing.T) {
		h, specSvc, fileSvc, analyticsSvc, _ := newHandler()
		viewerID := uuid.New()
		producerID := uuid.New()
		specID := uuid.New()

		fileSvc.On("GetKeyFromUrl", "").Return("", errors.New("no image")).Once()
		analyticsSvc.On("GetPublicAnalytics", mock.Anything, specID, &viewerID).Return(nil, errors.New("analytics down")).Once()

		specSvc.On("ListSpecs", mock.Anything, mock.MatchedBy(func(filter domain.SpecFilter) bool {
			return filter.Category == domain.Category("sample") &&
				filter.Search == "vocal" &&
				filter.Key == "" &&
				filter.MinBPM == 90 &&
				filter.MaxBPM == 140 &&
				filter.MinPrice == 10 &&
				filter.MaxPrice == 99 &&
				filter.Sort == "newest" &&
				filter.Offset == 0 &&
				filter.Limit == 20 &&
				len(filter.Genres) == 2 &&
				filter.Genres[0] == "hiphop" &&
				filter.Genres[1] == "drill" &&
				len(filter.Tags) == 2 &&
				filter.Tags[0] == "dark" &&
				filter.Tags[1] == "808"
		})).Return([]domain.Spec{{
			ID:         specID,
			ProducerID: producerID,
			Title:      "S",
			ImageUrl:   "",
		}}, 1, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/specs?category=sample&genres=hiphop,drill&tags=dark,808&search=vocal&key=All&min_bpm=90&max_bpm=140&min_price=10&max_price=99&page=0&sort=newest", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, viewerID))
		w := httptest.NewRecorder()
		h.List(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("get returns 404 when spec does not exist", func(t *testing.T) {
		h, specSvc, _, _, _ := newHandler()
		specID := uuid.New()

		specSvc.On("GetSpec", mock.Anything, specID).Return((*domain.Spec)(nil), nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/specs/"+specID.String(), nil)
		req.SetPathValue("id", specID.String())
		w := httptest.NewRecorder()
		h.Get(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("update covers get errors, missing spec, metadata and success", func(t *testing.T) {
		h, specSvc, fileSvc, _, _ := newHandler()
		specID := uuid.New()
		producerID := uuid.New()

		req := httptest.NewRequest(http.MethodPatch, "/specs/"+specID.String(), nil)
		req.SetPathValue("id", specID.String())
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
		w := httptest.NewRecorder()
		specSvc.On("GetSpec", mock.Anything, specID).Return(nil, errors.New("db")).Once()
		h.Update(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)

		req = httptest.NewRequest(http.MethodPatch, "/specs/"+specID.String(), nil)
		req.SetPathValue("id", specID.String())
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
		w = httptest.NewRecorder()
		specSvc.On("GetSpec", mock.Anything, specID).Return((*domain.Spec)(nil), nil).Once()
		h.Update(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)

		req = makeMultipartRequest(t, http.MethodPatch, "/specs/"+specID.String(), map[string]interface{}{"title": "T"})
		req.SetPathValue("id", specID.String())
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
		w = httptest.NewRecorder()
		specSvc.On("GetSpec", mock.Anything, specID).Return(&domain.Spec{ID: specID, ProducerID: producerID, Title: "Old"}, nil).Once()
		specSvc.On("UpdateSpec", mock.Anything, mock.AnythingOfType("*domain.Spec"), producerID).Return(nil).Once()
		fileSvc.On("GetKeyFromUrl", "").Return("", errors.New("no image")).Once()
		h.Update(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("update handles malformed multipart and invalid metadata json", func(t *testing.T) {
		h, specSvc, _, _, _ := newHandler()
		specID := uuid.New()
		producerID := uuid.New()

		req := httptest.NewRequest(http.MethodPatch, "/specs/"+specID.String(), bytes.NewBufferString("plain-body"))
		req.SetPathValue("id", specID.String())
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
		w := httptest.NewRecorder()
		specSvc.On("GetSpec", mock.Anything, specID).Return(&domain.Spec{ID: specID, ProducerID: producerID}, nil).Once()
		h.Update(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		var badBody bytes.Buffer
		writer := multipart.NewWriter(&badBody)
		assert.NoError(t, writer.WriteField("metadata", "{bad-json"))
		assert.NoError(t, writer.Close())
		req = httptest.NewRequest(http.MethodPatch, "/specs/"+specID.String(), &badBody)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.SetPathValue("id", specID.String())
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
		w = httptest.NewRecorder()
		specSvc.On("GetSpec", mock.Anything, specID).Return(&domain.Spec{ID: specID, ProducerID: producerID}, nil).Once()
		h.Update(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("delete covers unauthorized, forbidden and soft delete", func(t *testing.T) {
		h, specSvc, _, _, _ := newHandler()
		specID := uuid.New()
		ownerID := uuid.New()
		otherID := uuid.New()

		req := httptest.NewRequest(http.MethodDelete, "/specs/"+specID.String(), nil)
		req.SetPathValue("id", specID.String())
		w := httptest.NewRecorder()
		h.Delete(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)

		req = httptest.NewRequest(http.MethodDelete, "/specs/"+specID.String(), nil)
		req.SetPathValue("id", specID.String())
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, otherID))
		w = httptest.NewRecorder()
		specSvc.On("GetSpec", mock.Anything, specID).Return(&domain.Spec{ID: specID, ProducerID: ownerID}, nil).Once()
		h.Delete(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)

		req = httptest.NewRequest(http.MethodDelete, "/specs/"+specID.String(), nil)
		req.SetPathValue("id", specID.String())
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, ownerID))
		w = httptest.NewRecorder()
		specSvc.On("GetSpec", mock.Anything, specID).Return(&domain.Spec{ID: specID, ProducerID: ownerID}, nil).Once()
		specSvc.On("DeleteSpec", mock.Anything, specID, ownerID).Return(domain.ErrSpecSoftDeleted).Once()
		h.Delete(w, req)
		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("get user specs returns internal server error when service fails", func(t *testing.T) {
		h, specSvc, _, _, _ := newHandler()
		userID := uuid.New()

		req := httptest.NewRequest(http.MethodGet, "/users/"+userID.String()+"/specs?page=2", nil)
		req.SetPathValue("id", userID.String())
		w := httptest.NewRecorder()

		specSvc.On("GetUserSpecs", mock.Anything, userID, 2).Return(nil, 0, errors.New("db")).Once()
		h.GetUserSpecs(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("create succeeds for sample with metadata only", func(t *testing.T) {
		h, specSvc, fileSvc, _, notificationSvc := newHandler()
		producerID := uuid.New()

		req := makeMultipartRequest(t, http.MethodPost, "/specs", map[string]interface{}{
			"title":    "Sample Pack",
			"category": "sample",
		})
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
		w := httptest.NewRecorder()

		specSvc.On("CreateSpec", mock.Anything, mock.AnythingOfType("*domain.Spec")).Return(nil).Once()
		fileSvc.On("GetKeyFromUrl", "").Return("", errors.New("no image key")).Once()
		specSvc.On("UpdateFilesAndStatus", mock.Anything, mock.AnythingOfType("uuid.UUID"), mock.Anything, domain.ProcessingStatusCompleted).Return(nil).Maybe()
		notificationSvc.On("Create", mock.Anything, producerID, "Upload Complete", mock.Anything, mock.Anything).Return(nil).Maybe()

		h.Create(w, req)
		assert.Equal(t, http.StatusAccepted, w.Code)
	})

	t.Run("update returns bad request for invalid image bytes", func(t *testing.T) {
		h, specSvc, _, _, _ := newHandler()
		specID := uuid.New()
		producerID := uuid.New()

		req := makeMultipartRequestWithFile(t, http.MethodPatch, "/specs/"+specID.String(), map[string]interface{}{
			"title": "Track",
		}, "image", "x.txt", []byte("not-an-image"))
		req.SetPathValue("id", specID.String())
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
		w := httptest.NewRecorder()

		specSvc.On("GetSpec", mock.Anything, specID).Return(&domain.Spec{ID: specID, ProducerID: producerID}, nil).Once()

		h.Update(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("delete returns 500 when loading spec fails", func(t *testing.T) {
		h, specSvc, _, _, _ := newHandler()
		specID := uuid.New()
		userID := uuid.New()
		req := httptest.NewRequest(http.MethodDelete, "/specs/"+specID.String(), nil)
		req.SetPathValue("id", specID.String())
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
		w := httptest.NewRecorder()

		specSvc.On("GetSpec", mock.Anything, specID).Return(nil, errors.New("db")).Once()
		h.Delete(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("delete returns 404 when spec missing", func(t *testing.T) {
		h, specSvc, _, _, _ := newHandler()
		specID := uuid.New()
		userID := uuid.New()
		req := httptest.NewRequest(http.MethodDelete, "/specs/"+specID.String(), nil)
		req.SetPathValue("id", specID.String())
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
		w := httptest.NewRecorder()

		specSvc.On("GetSpec", mock.Anything, specID).Return((*domain.Spec)(nil), nil).Once()
		h.Delete(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
