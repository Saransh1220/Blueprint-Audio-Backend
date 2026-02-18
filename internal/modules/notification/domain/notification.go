package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type NotificationType string

const (
	NotificationTypeInfo    NotificationType = "info"
	NotificationTypeSuccess NotificationType = "success"
	NotificationTypeWarning NotificationType = "warning"
	NotificationTypeError   NotificationType = "error"
)

type Notification struct {
	ID        uuid.UUID        `json:"id" db:"id"`
	UserID    uuid.UUID        `json:"user_id" db:"user_id"`
	Title     string           `json:"title" db:"title"`
	Message   string           `json:"message" db:"message"`
	Type      NotificationType `json:"type" db:"type"`
	IsRead    bool             `json:"is_read" db:"is_read"`
	CreatedAt time.Time        `json:"created_at" db:"created_at"`
}

var (
	ErrNotificationNotFound = errors.New("notification not found")
)
