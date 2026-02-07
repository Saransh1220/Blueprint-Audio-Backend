package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/dto"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/saransh1220/blueprint-audio/internal/service"
)

type UserHandler struct {
	service     service.UserService
	fileService service.FileService
}

func NewUserHandler(service service.UserService, fileService service.FileService) *UserHandler {
	return &UserHandler{
		service:     service,
		fileService: fileService,
	}
}

// UpdateProfile handles PATCH /users/profile - updates authenticated user's profile
func (h *UserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req dto.UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if err := h.service.UpdateProfile(r.Context(), userID, req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return updated profile
	profile, err := h.service.GetPublicProfile(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

// GetPublicProfile handles GET /users/:id/public - gets a user's public profile
func (h *UserHandler) GetPublicProfile(w http.ResponseWriter, r *http.Request) {
	userIDStr := r.PathValue("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}

	profile, err := h.service.GetPublicProfile(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Sanitize profile to generate presigned URLs for avatar
	h.sanitizeUserProfile(profile)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

// UploadAvatar handles POST /users/profile/avatar - uploads a new avatar image
func (h *UserHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Limit request size to 10MB
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	// Parse multipart form (max 10MB for avatar)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "file too large", http.StatusBadRequest)
		return
	}

	// Get the avatar file
	file, header, err := r.FormFile("avatar")
	if err != nil {
		http.Error(w, "avatar file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Get current user profile to check for existing avatar
	currentUser, err := h.service.GetPublicProfile(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to get user", http.StatusInternalServerError)
		return
	}

	// Delete old avatar if it exists
	if currentUser.AvatarURL != nil && *currentUser.AvatarURL != "" {
		oldKey, err := h.fileService.GetKeyFromUrl(*currentUser.AvatarURL)
		if err == nil {
			// Ignore deletion errors (file might not exist)
			_ = h.fileService.Delete(r.Context(), oldKey)
		}
	}

	// Upload to S3
	avatarURL, _, err := h.fileService.Upload(r.Context(), file, header, "avatars")
	if err != nil {
		http.Error(w, "failed to upload avatar: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update user profile with new avatar URL
	req := dto.UpdateProfileRequest{
		AvatarURL: &avatarURL,
	}
	if err := h.service.UpdateProfile(r.Context(), userID, req); err != nil {
		// Rollback: delete the newly uploaded file
		if newKey, keyErr := h.fileService.GetKeyFromUrl(avatarURL); keyErr == nil {
			_ = h.fileService.Delete(r.Context(), newKey)
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return updated profile
	profile, err := h.service.GetPublicProfile(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Sanitize to generate presigned URL
	h.sanitizeUserProfile(profile)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

// sanitizeUserProfile generates presigned URLs for avatar images
func (h *UserHandler) sanitizeUserProfile(profile *dto.PublicUserResponse) {
	if profile.AvatarURL == nil || *profile.AvatarURL == "" {
		return
	}

	key, err := h.fileService.GetKeyFromUrl(*profile.AvatarURL)
	if err != nil {
		return // Keep original URL if we can't parse it
	}

	presignedURL, err := h.fileService.GetPresignedURL(context.Background(), key, time.Hour)
	if err == nil && presignedURL != "" {
		profile.AvatarURL = &presignedURL
	}
}
