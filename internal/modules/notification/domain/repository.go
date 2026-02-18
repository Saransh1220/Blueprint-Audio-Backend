package domain

import (
	"context"

	"github.com/google/uuid"
)

type NotificationRepository interface {
	Create(ctx context.Context, notification *Notification) error
	GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]Notification, error)
	MarkAsRead(ctx context.Context, notificationID, userID uuid.UUID) error
	MarkAllAsRead(ctx context.Context, userID uuid.UUID) error
	UnreadCount(ctx context.Context, userID uuid.UUID) (int, error)
}
