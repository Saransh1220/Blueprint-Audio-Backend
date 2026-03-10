package http

import (
	"context"
	"encoding/json"
	"errors"
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
	Login(ctx context.Context, req application.LoginRequest) (*application.TokenPair, error)
	GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GoogleLogin(ctx context.Context, googleClientID string, req application.GoogleLoginRequest) (*application.TokenPair, error)
	RefreshSession(ctx context.Context, refreshToken string) (string, error)
	Logout(ctx context.Context, refreshToken string) error
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
	refreshExpiry  time.Duration
}

func NewAuthHandler(service AuthService, fileService FileService, googleClientID string, refreshExpiry time.Duration) *AuthHandler {
	return &AuthHandler{
		service:        service,
		fileService:    fileService,
		googleClientID: googleClientID,
		refreshExpiry:  refreshExpiry,
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

	tokens, err := h.service.Login(r.Context(), req)
	if err != nil {
		if err == domain.ErrInvalidCredentials {
			http.Error(w, `{"error": "invalid credentials"}`, http.StatusUnauthorized)
			return
		}
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	// Set HTTP-Only Cookie for the Refresh Token
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    tokens.RefreshToken,
		Path:     "/",
		Expires:  time.Now().Add(h.refreshExpiry),
		HttpOnly: true,
		Secure:   true, // Secure in production (HTTPS)
		SameSite: http.SameSiteStrictMode,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"token": tokens.AccessToken})
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

	tokens, err := h.service.GoogleLogin(r.Context(), h.googleClientID, req)
	if err != nil {
		log.Printf("GoogleLogin Auth Service Error: %v", err)
		if errors.Is(err, application.ErrGoogleAuthFailed) {
			http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusUnauthorized)
			return
		}
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	log.Printf("GoogleLogin Success!")
	// Set HTTP-Only Cookie for the Refresh Token
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    tokens.RefreshToken,
		Path:     "/",
		Expires:  time.Now().Add(h.refreshExpiry),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"token": tokens.AccessToken})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read refresh token from HttpOnly cookie
	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		// Log specific error for debugging
		log.Printf("Refresh Token Error: Missing cookie - %v", err)
		http.Error(w, `{"error": "refresh token missing"}`, http.StatusUnauthorized)
		return
	}

	newAccessToken, err := h.service.RefreshSession(r.Context(), cookie.Value)
	if err != nil {
		log.Printf("Refresh Error (Service): %v", err)
		// Specifically check for specific domain errors if necessary
		http.Error(w, `{"error": "invalid or expired refresh token"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"token": newAccessToken})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read refresh token (if it exists)
	cookie, err := r.Cookie("refresh_token")
	if err == nil && cookie.Value != "" {
		// Invalidate session in DB
		err = h.service.Logout(r.Context(), cookie.Value)
		if err != nil {
			log.Printf("Failed to revoke session: %v", err)
			// Continue to clear cookie even if DB update fails
		}
	}

	// Clear the refresh_token cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0), // Expire immediately
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "logged out successfully"})
}
