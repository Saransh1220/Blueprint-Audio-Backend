package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/legal/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/legal/domain"
)

type LegalHandler struct {
	service application.LegalService
}

func NewLegalHandler(service application.LegalService) *LegalHandler {
	return &LegalHandler{service: service}
}

func (h *LegalHandler) GetDocument(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		http.Error(w, "missing document key", http.StatusBadRequest)
		return
	}

	locale := r.URL.Query().Get("locale")
	view, err := h.service.GetDocument(r.Context(), key, locale)
	if err != nil {
		if errors.Is(err, domain.ErrDocumentNotFound) || errors.Is(err, domain.ErrVersionNotFound) {
			http.Error(w, "document not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	json.NewEncoder(w).Encode(view)
}

func (h *LegalHandler) GetDocumentVersions(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		http.Error(w, "missing document key", http.StatusBadRequest)
		return
	}

	versions, err := h.service.GetDocumentVersions(r.Context(), key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": versions})
}

func (h *LegalHandler) GetDocumentVersion(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	version := r.PathValue("version")
	if key == "" || version == "" {
		http.Error(w, "missing document key or version", http.StatusBadRequest)
		return
	}

	ver, err := h.service.GetDocumentVersion(r.Context(), key, version)
	if err != nil {
		if errors.Is(err, domain.ErrVersionNotFound) {
			http.Error(w, "version not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	json.NewEncoder(w).Encode(ver)
}

func (h *LegalHandler) GetRequirements(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	reqs, err := h.service.GetRequirements(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": reqs})
}

type RecordAcceptanceBody struct {
	DocumentKey string    `json:"document_key"`
	VersionID   uuid.UUID `json:"version_id"`
	ContentHash string    `json:"content_hash"`
	Action      string    `json:"action"`
	Context     string    `json:"context"`
}

func (h *LegalHandler) RecordAcceptance(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var body RecordAcceptanceBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if body.DocumentKey == "" || body.VersionID == uuid.Nil {
		http.Error(w, "document_key and version_id are required", http.StatusBadRequest)
		return
	}

	if body.Action == "" {
		body.Action = "acknowledgement"
	}

	err := h.service.RecordAcceptance(r.Context(), userID, body.DocumentKey, body.VersionID, body.ContentHash, body.Action, body.Context)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidHash) {
			http.Error(w, "stale or mismatched document hash", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *LegalHandler) CreateDraft(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	key := r.PathValue("key")
	var draft domain.DocumentVersion
	if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	draft.DocumentKey = key

	if err := h.service.CreateDraft(r.Context(), userID, &draft); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(draft)
}

func (h *LegalHandler) PublishVersion(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var body struct {
		VersionID uuid.UUID `json:"version_id"`
		Notes     string    `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.service.PublishVersion(r.Context(), userID, body.VersionID, body.Notes); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *LegalHandler) GetAudits(w http.ResponseWriter, r *http.Request) {
	audits, err := h.service.GetAudits(r.Context(), 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": audits})
}
