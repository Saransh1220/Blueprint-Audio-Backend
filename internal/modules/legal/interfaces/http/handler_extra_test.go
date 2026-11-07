package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/legal/domain"
	"github.com/stretchr/testify/assert"
)

func authedLegalReq(method, path string, body any) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	r := httptest.NewRequest(method, path, &buf)
	ctx := context.WithValue(r.Context(), middleware.ContextKeyUserId, uuid.New())
	return r.WithContext(ctx)
}

func TestLegalHandler_RecordAcceptance_Extra(t *testing.T) {
	verID := uuid.New()

	t.Run("unauthorized", func(t *testing.T) {
		h := NewLegalHandler(&mockLegalService{})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/legal/accept", nil)
		h.RecordAcceptance(w, r)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("bad json", func(t *testing.T) {
		h := NewLegalHandler(&mockLegalService{})
		w := httptest.NewRecorder()
		r := authedLegalReq(http.MethodPost, "/legal/accept", nil)
		r.Body = http.NoBody
		h.RecordAcceptance(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing required fields", func(t *testing.T) {
		h := NewLegalHandler(&mockLegalService{})
		w := httptest.NewRecorder()
		r := authedLegalReq(http.MethodPost, "/legal/accept", map[string]any{"document_key": ""})
		h.RecordAcceptance(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("stale hash conflict", func(t *testing.T) {
		h := NewLegalHandler(&mockLegalService{err: domain.ErrInvalidHash})
		w := httptest.NewRecorder()
		r := authedLegalReq(http.MethodPost, "/legal/accept", RecordAcceptanceBody{
			DocumentKey: "terms",
			VersionID:   verID,
			ContentHash: "stale",
		})
		h.RecordAcceptance(w, r)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		svc := &mockLegalService{}
		h := NewLegalHandler(svc)
		w := httptest.NewRecorder()
		r := authedLegalReq(http.MethodPost, "/legal/accept", RecordAcceptanceBody{
			DocumentKey: "terms",
			VersionID:   verID,
			ContentHash: "fresh",
		})
		h.RecordAcceptance(w, r)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "terms", svc.acceptedKey)
		assert.Equal(t, verID, svc.acceptedVer)
	})
}

func TestLegalHandler_DraftAndPublish(t *testing.T) {
	verID := uuid.New()

	t.Run("CreateDraft unauthorized and bad body", func(t *testing.T) {
		h := NewLegalHandler(&mockLegalService{})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/legal/terms/draft", nil)
		h.CreateDraft(w, r)
		assert.Equal(t, http.StatusUnauthorized, w.Code)

		w = httptest.NewRecorder()
		r = authedLegalReq(http.MethodPost, "/legal/terms/draft", nil)
		r.Body = http.NoBody
		h.CreateDraft(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("CreateDraft success", func(t *testing.T) {
		h := NewLegalHandler(&mockLegalService{})
		w := httptest.NewRecorder()
		r := authedLegalReq(http.MethodPost, "/legal/terms/draft", domain.DocumentVersion{
			Title: "New terms",
		})
		r.SetPathValue("key", "terms")
		h.CreateDraft(w, r)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("PublishVersion unauthorized and success", func(t *testing.T) {
		h := NewLegalHandler(&mockLegalService{})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/legal/publish", nil)
		h.PublishVersion(w, r)
		assert.Equal(t, http.StatusUnauthorized, w.Code)

		w = httptest.NewRecorder()
		r = authedLegalReq(http.MethodPost, "/legal/publish", map[string]any{"version_id": verID, "notes": "published"})
		h.PublishVersion(w, r)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GetAudits error and success", func(t *testing.T) {
		h := NewLegalHandler(&mockLegalService{err: errors.New("audit err")})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/legal/audits", nil)
		h.GetAudits(w, r)
		assert.Equal(t, http.StatusInternalServerError, w.Code)

		h = NewLegalHandler(&mockLegalService{audits: []domain.PublicationAudit{{ID: uuid.New()}}})
		w = httptest.NewRecorder()
		r = httptest.NewRequest(http.MethodGet, "/legal/audits", nil)
		h.GetAudits(w, r)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}
