package handler

import (
	"encoding/json"
	"net/http"

	"github.com/saransh1220/blueprint-audio/internal/domain"

	"github.com/saransh1220/blueprint-audio/internal/service"
)

type AuthHandler struct {
	service service.AuthServiceInterface
}

func NewAuthHandler(service service.AuthServiceInterface) *AuthHandler {
	return &AuthHandler{service: service}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req service.RegisterUserReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid input: "+err.Error(), http.StatusBadRequest)
		return
	}

	// 3. Call Service
	user, err := h.service.RegisterUser(r.Context(), req)
	if err != nil {
		if err == domain.ErrUserAlreadyExists {
			http.Error(w, `{"error": "user already exists"}`, http.StatusConflict)
			return
		}
		// In production, we'd check for validation errors specifically.
		// For now, we assume other errors are bad request/validation.
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	// 4. Send Response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(user); err != nil {
		http.Error(w, `{"error": "failed to encode response"}`, http.StatusInternalServerError)
	}

}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req service.LoginUserReq

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	token, err := h.service.LoginUser(r.Context(), req)
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
