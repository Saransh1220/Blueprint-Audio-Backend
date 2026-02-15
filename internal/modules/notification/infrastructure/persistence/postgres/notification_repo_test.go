package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/notification/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/notification/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPgNotificationRepository_CRUDLikeOperations(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := postgres.NewPgNotificationRepository(db)
	ctx := context.Background()
	userID := uuid.New()
	notificationID := uuid.New()

	n := &domain.Notification{
		ID:        notificationID,
		UserID:    userID,
		Title:     "Title",
		Message:   "Message",
		Type:      domain.NotificationTypeInfo,
		IsRead:    false,
		CreatedAt: time.Now(),
	}

	mock.ExpectExec(`INSERT INTO notifications`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.Create(ctx, n))

	rows := sqlmock.NewRows([]string{"id", "user_id", "title", "message", "type", "is_read", "created_at"}).
		AddRow(notificationID, userID, "Title", "Message", "info", false, time.Now())
	mock.ExpectQuery(`SELECT \* FROM notifications`).
		WithArgs(userID, 10, 5).
		WillReturnRows(rows)
	items, err := repo.GetByUserID(ctx, userID, 10, 5)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, userID, items[0].UserID)

	mock.ExpectExec(`UPDATE notifications`).
		WithArgs(notificationID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.MarkAsRead(ctx, notificationID))

	mock.ExpectExec(`UPDATE notifications`).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 2))
	require.NoError(t, repo.MarkAllAsRead(ctx, userID))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM notifications`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	count, err := repo.UnreadCount(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	require.NoError(t, mock.ExpectationsWereMet())
}
