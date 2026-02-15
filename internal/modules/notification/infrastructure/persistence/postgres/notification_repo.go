package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/notification/domain"
)

type PgNotificationRepository struct {
	db *sqlx.DB
}

func NewPgNotificationRepository(db *sqlx.DB) *PgNotificationRepository {
	return &PgNotificationRepository{db: db}
}

func (r *PgNotificationRepository) Create(ctx context.Context, n *domain.Notification) error {
	query := `
		INSERT INTO notifications (id, user_id, title, message, type, is_read, created_at)
		VALUES (:id, :user_id, :title, :message, :type, :is_read, :created_at)
	`
	_, err := r.db.NamedExecContext(ctx, query, n)
	return err
}

func (r *PgNotificationRepository) GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.Notification, error) {
	query := `
		SELECT * FROM notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	var notifications []domain.Notification
	err := r.db.SelectContext(ctx, &notifications, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	return notifications, nil
}

func (r *PgNotificationRepository) MarkAsRead(ctx context.Context, notificationID uuid.UUID) error {
	query := `
		UPDATE notifications
		SET is_read = TRUE
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query, notificationID)
	return err
}

func (r *PgNotificationRepository) MarkAllAsRead(ctx context.Context, userID uuid.UUID) error {
	query := `
		UPDATE notifications
		SET is_read = TRUE
		WHERE user_id = $1
	`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

func (r *PgNotificationRepository) UnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	query := `
		SELECT COUNT(*) FROM notifications
		WHERE user_id = $1 AND is_read = FALSE
	`
	var count int
	err := r.db.GetContext(ctx, &count, query, userID)
	return count, err
}
