package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSpecHandler_DownloadFree_Branches(t *testing.T) {
	h, specSvc, fileSvc, analyticsSvc := newHandler()
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
	h, specSvc, _, _ := newHandler()
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
