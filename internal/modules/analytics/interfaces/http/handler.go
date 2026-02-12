package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/analytics/application"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
)

type AnalyticsHandler struct {
	service     application.AnalyticsService
	specRepo    catalogDomain.SpecRepository // Need this for ownership checks if not fully encapsulated in service
	fileService FileService                  // Interface for file operations if needed (e.g. downloads)
}

// FileService interface for download tracking dependencies if any
type FileService interface {
	GetPresignedDownloadURL(ctx context.Context, key string, filename string, expiration time.Duration) (string, error)
	GetKeyFromUrl(url string) (string, error)
}

func NewAnalyticsHandler(service application.AnalyticsService, specRepo catalogDomain.SpecRepository, fileService FileService) *AnalyticsHandler {
	return &AnalyticsHandler{
		service:     service,
		specRepo:    specRepo,
		fileService: fileService,
	}
}

func (h *AnalyticsHandler) TrackPlay(w http.ResponseWriter, r *http.Request) {
	specIDStr := r.PathValue("id")
	specID, err := uuid.Parse(specIDStr)
	if err != nil {
		http.Error(w, "invalid spec id", http.StatusBadRequest)
		return
	}

	if err := h.service.TrackPlay(r.Context(), specID); err != nil {
		http.Error(w, "failed to track play", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *AnalyticsHandler) ToggleFavorite(w http.ResponseWriter, r *http.Request) {
	specIDStr := r.PathValue("id")
	specID, err := uuid.Parse(specIDStr)
	if err != nil {
		http.Error(w, "invalid spec id", http.StatusBadRequest)
		return
	}

	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	isFavorited, err := h.service.ToggleFavorite(r.Context(), userID, specID)
	if err != nil {
		http.Error(w, "failed to toggle favorite", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"is_favorited": isFavorited,
	})
}

func (h *AnalyticsHandler) GetProducerAnalytics(w http.ResponseWriter, r *http.Request) {
	specIDStr := r.PathValue("id")
	specID, err := uuid.Parse(specIDStr)
	if err != nil {
		http.Error(w, "invalid spec id", http.StatusBadRequest)
		return
	}

	producerID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	analytics, err := h.service.GetProducerAnalytics(r.Context(), specID, producerID)
	if err != nil {
		status := http.StatusInternalServerError
		msg := "failed to fetch producer analytics"
		switch {
		case strings.Contains(strings.ToLower(err.Error()), "unauthorized"):
			status = http.StatusForbidden
			msg = "forbidden"
		case errors.Is(err, catalogDomain.ErrSpecNotFound), strings.Contains(strings.ToLower(err.Error()), "not found"):
			status = http.StatusNotFound
			msg = "spec not found"
		}
		http.Error(w, msg, status)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analytics)
}

func (h *AnalyticsHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	producerID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	daysStr := r.URL.Query().Get("days")
	days := 30
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil {
			days = d
		}
	}

	sortBy := r.URL.Query().Get("sort")

	stats, err := h.service.GetStatsOverview(r.Context(), producerID, days, sortBy)
	if err != nil {
		http.Error(w, "failed to fetch analytics overview", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *AnalyticsHandler) GetTopSpecs(w http.ResponseWriter, r *http.Request) {
	producerID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	limit := 5
	sortBy := r.URL.Query().Get("sortBy")

	stats, err := h.service.GetTopSpecs(r.Context(), producerID, limit, sortBy)
	if err != nil {
		http.Error(w, "failed to fetch top specs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
