package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/notification/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/notification/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/notification/infrastructure/websocket"
)

type NotificationHandler struct {
	service *application.NotificationService
	hub     *websocket.Hub
}

func NewNotificationHandler(service *application.NotificationService, hub *websocket.Hub) *NotificationHandler {
	return &NotificationHandler{service: service, hub: hub}
}

func (h *NotificationHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	// userId is extracted from context by AuthMiddleware
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		// If no user ID, maybe unauthorized or anonymous?
		// For notifications, we probably want authenticated users.
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	websocket.ServeWs(h.hub, w, r, userID)
}

func (h *NotificationHandler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	log.Println("ListNotifications: starting")
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	log.Printf("ListNotifications: userID=%s", userID)

	limit := 20
	offset := 0

	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	log.Println("ListNotifications: calling service")
	notifications, err := h.service.GetUserNotifications(r.Context(), userID, limit, offset)
	if err != nil {
		log.Printf("ListNotifications: service error: %v", err)
		http.Error(w, "failed to fetch notifications", http.StatusInternalServerError)
		return
	}

	log.Printf("ListNotifications: found %d notifications", len(notifications))

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"data": notifications}); err != nil {
		log.Printf("ListNotifications: encode error: %v", err)
	}
}

func (h *NotificationHandler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	notificationIDStr := r.PathValue("id")
	notificationID, err := uuid.Parse(notificationIDStr)
	if err != nil {
		http.Error(w, "invalid notification id", http.StatusBadRequest)
		return
	}

	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := h.service.MarkAsRead(r.Context(), notificationID, userID); err != nil {
		if errors.Is(err, domain.ErrNotificationNotFound) {
			http.Error(w, "notification not found or unauthorized", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to mark notification as read", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *NotificationHandler) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := h.service.MarkAllAsRead(r.Context(), userID); err != nil {
		http.Error(w, "failed to mark all notifications as read", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *NotificationHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	count, err := h.service.UnreadCount(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to get unread count", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"count": count})
}
