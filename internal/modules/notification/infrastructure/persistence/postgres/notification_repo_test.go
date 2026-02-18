package postgres_test

import (
	"context"
	"errors"
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
		WithArgs(notificationID, userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.MarkAsRead(ctx, notificationID, userID))

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

func TestPgNotificationRepository_Create_SetsCreatedAtWhenZero(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := postgres.NewPgNotificationRepository(db)
	ctx := context.Background()

	n := &domain.Notification{
		ID:      uuid.New(),
		UserID:  uuid.New(),
		Title:   "T",
		Message: "M",
		Type:    domain.NotificationTypeInfo,
		IsRead:  false,
	}
	require.True(t, n.CreatedAt.IsZero())

	mock.ExpectExec(`INSERT INTO notifications`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.Create(ctx, n))
	assert.False(t, n.CreatedAt.IsZero())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPgNotificationRepository_GetByUserID_Error(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := postgres.NewPgNotificationRepository(db)
	ctx := context.Background()
	userID := uuid.New()

	mock.ExpectQuery(`SELECT \* FROM notifications`).
		WithArgs(userID, 10, 0).
		WillReturnError(errors.New("query fail"))

	items, err := repo.GetByUserID(ctx, userID, 10, 0)
	require.Error(t, err)
	assert.Nil(t, items)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPgNotificationRepository_MarkAsRead_ErrorBranches(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := postgres.NewPgNotificationRepository(db)
	ctx := context.Background()
	notificationID := uuid.New()
	userID := uuid.New()

	t.Run("exec error", func(t *testing.T) {
		mock.ExpectExec(`UPDATE notifications`).
			WithArgs(notificationID, userID).
			WillReturnError(errors.New("exec fail"))
		err := repo.MarkAsRead(ctx, notificationID, userID)
		require.EqualError(t, err, "exec fail")
	})

	t.Run("rows affected error", func(t *testing.T) {
		mock.ExpectExec(`UPDATE notifications`).
			WithArgs(notificationID, userID).
			WillReturnResult(sqlmock.NewErrorResult(errors.New("rows fail")))
		err := repo.MarkAsRead(ctx, notificationID, userID)
		require.EqualError(t, err, "rows fail")
	})

	t.Run("not found", func(t *testing.T) {
		mock.ExpectExec(`UPDATE notifications`).
			WithArgs(notificationID, userID).
			WillReturnResult(sqlmock.NewResult(0, 0))
		err := repo.MarkAsRead(ctx, notificationID, userID)
		require.ErrorIs(t, err, domain.ErrNotificationNotFound)
	})

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPgNotificationRepository_UnreadCount_Error(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := postgres.NewPgNotificationRepository(db)
	ctx := context.Background()
	userID := uuid.New()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM notifications`).
		WithArgs(userID).
		WillReturnError(errors.New("count fail"))

	count, err := repo.UnreadCount(ctx, userID)
	require.EqualError(t, err, "count fail")
	assert.Equal(t, 0, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPgNotificationRepository_MarkAllAsRead_Error(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := postgres.NewPgNotificationRepository(db)
	ctx := context.Background()
	userID := uuid.New()

	mock.ExpectExec(`UPDATE notifications`).
		WithArgs(userID).
		WillReturnError(errors.New("exec fail"))

	err := repo.MarkAllAsRead(ctx, userID)
	require.EqualError(t, err, "exec fail")
	require.NoError(t, mock.ExpectationsWereMet())
}
