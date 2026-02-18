package application

import (
	"context"
	"time"

	"encoding/json"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/notification/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/notification/infrastructure/websocket"
)

type NotificationService struct {
	repo domain.NotificationRepository
	hub  *websocket.Hub
}

func NewNotificationService(repo domain.NotificationRepository, hub *websocket.Hub) *NotificationService {
	return &NotificationService{repo: repo, hub: hub}
}

func (s *NotificationService) Create(ctx context.Context, userID uuid.UUID, title, message string, type_ domain.NotificationType) error {
	notification := &domain.Notification{
		ID:        uuid.New(),
		UserID:    userID,
		Title:     title,
		Message:   message,
		Type:      type_,
		IsRead:    false,
		CreatedAt: time.Now(),
	}
	err := s.repo.Create(ctx, notification)
	if err != nil {
		return err
	}

	// Broadcast to WebSocket
	// We might want to optimize this to only send to specific user, but for now hub broadcasts to all (filtered by client side? No, hub has all clients)
	// Actually, my Hub implementation broadcasts to ALL clients.
	// We need targeted notifications.
	// My Client struct has UserID.
	// I should use that.

	// For now, let's just broadcast and let the client implementation handle filtering or update Hub to support direct messaging.
	// The current Hub implementation broadcasts to everyone.
	// I'll update the Hub later or now to support filtering, but let's stick to the plan.
	// Sending the notification as JSON.

	msgBytes, err := json.Marshal(notification)
	if err == nil {
		// s.hub.BroadcastMessage(msgBytes) // OLD: Insecure broadcast
		s.hub.SendToUser(userID, msgBytes) // NEW: Secure unicast
	}

	return nil
}

func (s *NotificationService) GetHub() *websocket.Hub {
	return s.hub
}

func (s *NotificationService) GetUserNotifications(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.Notification, error) {
	return s.repo.GetByUserID(ctx, userID, limit, offset)
}

func (s *NotificationService) MarkAsRead(ctx context.Context, notificationID, userID uuid.UUID) error {
	return s.repo.MarkAsRead(ctx, notificationID, userID)
}

func (s *NotificationService) MarkAllAsRead(ctx context.Context, userID uuid.UUID) error {
	return s.repo.MarkAllAsRead(ctx, userID)
}

func (s *NotificationService) UnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	return s.repo.UnreadCount(ctx, userID)
}
