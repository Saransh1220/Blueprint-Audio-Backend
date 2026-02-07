package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/dto"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/saransh1220/blueprint-audio/internal/service"
)

type AnalyticsHandler struct {
	analyticsService service.AnalyticsServiceInterface
	specRepo         domain.SpecRepository
}

func NewAnalyticsHandler(analyticsService service.AnalyticsServiceInterface, specRepo domain.SpecRepository) *AnalyticsHandler {
	return &AnalyticsHandler{
		analyticsService: analyticsService,
		specRepo:         specRepo,
	}
}

// TrackPlay increments play count for a spec
func (h *AnalyticsHandler) TrackPlay(w http.ResponseWriter, r *http.Request) {
	specID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid spec ID", http.StatusBadRequest)
		return
	}

	err = h.analyticsService.TrackPlay(r.Context(), specID)
	if err != nil {
		http.Error(w, "Failed to track play", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// ToggleFavorite adds or removes a spec from user's favorites
func (h *AnalyticsHandler) ToggleFavorite(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context (set by auth middleware)
	userIDInterface, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	specID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid spec ID", http.StatusBadRequest)
		return
	}

	isFavorited, err := h.analyticsService.ToggleFavorite(r.Context(), userIDInterface, specID)
	if err != nil {
		http.Error(w, "Failed to toggle favorite", http.StatusInternalServerError)
		return
	}

	// Get updated favorite count
	analytics, err := h.analyticsService.GetPublicAnalytics(r.Context(), specID, nil)
	favoriteCount := 0
	if err == nil && analytics != nil {
		favoriteCount = analytics.FavoriteCount
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"favorited":   isFavorited,
		"total_count": favoriteCount,
	})
}

// GetProducerAnalytics returns detailed analytics for a spec (producer only)
func (h *AnalyticsHandler) GetProducerAnalytics(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context
	userIDInterface, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	specID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid spec ID", http.StatusBadRequest)
		return
	}

	analytics, err := h.analyticsService.GetProducerAnalytics(r.Context(), specID, userIDInterface)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	response := dto.ProducerAnalytics{
		PlayCount:          analytics.PlayCount,
		FavoriteCount:      analytics.FavoriteCount,
		TotalDownloadCount: analytics.TotalDownloadCount,
		TotalPurchaseCount: analytics.TotalPurchaseCount,
		PurchasesByLicense: analytics.PurchasesByLicense,
		TotalRevenue:       analytics.TotalRevenue,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// DownloadFreeMp3 provides free MP3 download if enabled
func (h *AnalyticsHandler) DownloadFreeMp3(w http.ResponseWriter, r *http.Request) {
	specID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid spec ID", http.StatusBadRequest)
		return
	}

	// Get spec to check if free download is enabled
	spec, err := h.specRepo.GetByID(r.Context(), specID)
	if err != nil {
		http.Error(w, "Spec not found", http.StatusNotFound)
		return
	}

	if !spec.FreeMp3Enabled {
		http.Error(w, "Free download not enabled for this spec", http.StatusForbidden)
		return
	}

	// Track the download
	err = h.analyticsService.TrackFreeDownload(r.Context(), specID)
	if err != nil {
		http.Error(w, "Failed to track download", http.StatusInternalServerError)
		return
	}

	// Return the preview URL (which is the MP3)
	// In a production system, you might want to generate a signed URL with expiry
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"download_url": spec.PreviewUrl,
		"message":      "Free MP3 download tracked successfully",
	})
}

// GetOverview returns aggregated analytics for the authenticated producer
func (h *AnalyticsHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context
	userIDInterface, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// In this app, producers view their own stats.
	// We assume the user ID is the producer ID.
	producerID := userIDInterface

	// Get days from query param (default 30)
	days := 30
	if val := r.URL.Query().Get("days"); val != "" {
		if d, err := strconv.Atoi(val); err == nil && d > 0 {
			days = d
		}
	}

	stats, err := h.analyticsService.GetStatsOverview(r.Context(), producerID, days)
	if err != nil {
		http.Error(w, "Failed to get analytics overview", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
