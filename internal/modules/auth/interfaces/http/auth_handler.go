package http

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
)

// AuthService defines the interface for auth operations
type AuthService interface {
	Register(ctx context.Context, req application.RegisterRequest) (*domain.User, error)
	Login(ctx context.Context, req application.LoginRequest) (string, error)
	GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GoogleLogin(ctx context.Context, googleClientID string, req application.GoogleLoginRequest) (string, error)
}

// FileService defines the interface for file operations
type FileService interface {
	GetKeyFromUrl(fileUrl string) (string, error)
	GetPresignedURL(ctx context.Context, objectName string, expiry time.Duration) (string, error)
}

type AuthHandler struct {
	service        AuthService
	fileService    FileService
	googleClientID string
}

func NewAuthHandler(service AuthService, fileService FileService, googleClientID string) *AuthHandler {
	return &AuthHandler{
		service:        service,
		fileService:    fileService,
		googleClientID: googleClientID,
	}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req application.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid input: "+err.Error(), http.StatusBadRequest)
		return
	}

	user, err := h.service.Register(r.Context(), req)
	if err != nil {
		if err == domain.ErrUserAlreadyExists {
			http.Error(w, `{"error": "user already exists"}`, http.StatusConflict)
			return
		}
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(user); err != nil {
		http.Error(w, `{"error": "failed to encode response"}`, http.StatusInternalServerError)
	}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req application.LoginRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	token, err := h.service.Login(r.Context(), req)
	if err != nil {
		if err == domain.ErrInvalidCredentials {
			http.Error(w, `{"error": "invalid credentials"}`, http.StatusUnauthorized)
			return
		}
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userId, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, `{"error": "user not authenticated"}`, http.StatusUnauthorized)
		return
	}

	user, err := h.service.GetUser(r.Context(), userId)
	if err != nil {
		if err == domain.ErrUserNotFound {
			http.Error(w, `{"error": "user not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	// Generate presigned URL for avatar if present
	if user.AvatarUrl != nil && *user.AvatarUrl != "" {
		key, err := h.fileService.GetKeyFromUrl(*user.AvatarUrl)
		if err == nil {
			presignedURL, err := h.fileService.GetPresignedURL(r.Context(), key, 3600*time.Second)
			if err == nil {
				user.AvatarUrl = &presignedURL
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (h *AuthHandler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	var req application.GoogleLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("GoogleLogin Error: invalid request body - %v", err)
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	log.Printf("GoogleLogin Request Received: token length = %d", len(req.Token))

	token, err := h.service.GoogleLogin(r.Context(), h.googleClientID, req)
	if err != nil {
		log.Printf("GoogleLogin Auth Service Error: %v", err)
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusUnauthorized)
		return
	}

	log.Printf("GoogleLogin Success!")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}
