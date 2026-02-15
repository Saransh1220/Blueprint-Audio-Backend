package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/notification/domain"
	ws "github.com/saransh1220/blueprint-audio/internal/modules/notification/infrastructure/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type notificationRepoMock struct {
	createFn        func(context.Context, *domain.Notification) error
	getByUserIDFn   func(context.Context, uuid.UUID, int, int) ([]domain.Notification, error)
	markAsReadFn    func(context.Context, uuid.UUID, uuid.UUID) error
	markAllAsReadFn func(context.Context, uuid.UUID) error
	unreadCountFn   func(context.Context, uuid.UUID) (int, error)
}

func (m notificationRepoMock) Create(ctx context.Context, n *domain.Notification) error {
	return m.createFn(ctx, n)
}

func (m notificationRepoMock) GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.Notification, error) {
	return m.getByUserIDFn(ctx, userID, limit, offset)
}

func (m notificationRepoMock) MarkAsRead(ctx context.Context, notificationID, userID uuid.UUID) error {
	return m.markAsReadFn(ctx, notificationID, userID)
}

func (m notificationRepoMock) MarkAllAsRead(ctx context.Context, userID uuid.UUID) error {
	return m.markAllAsReadFn(ctx, userID)
}

func (m notificationRepoMock) UnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	return m.unreadCountFn(ctx, userID)
}

func TestNotificationService_Create(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		hub := ws.NewHub()
		go hub.Run()
		defer hub.Stop()

		userID := uuid.New()
		var captured *domain.Notification
		repo := notificationRepoMock{
			createFn: func(_ context.Context, n *domain.Notification) error {
				captured = n
				return nil
			},
			getByUserIDFn:   func(context.Context, uuid.UUID, int, int) ([]domain.Notification, error) { return nil, nil },
			markAsReadFn:    func(context.Context, uuid.UUID, uuid.UUID) error { return nil },
			markAllAsReadFn: func(context.Context, uuid.UUID) error { return nil },
			unreadCountFn:   func(context.Context, uuid.UUID) (int, error) { return 0, nil },
		}
		svc := NewNotificationService(repo, hub)

		err := svc.Create(context.Background(), userID, "Title", "Message", domain.NotificationTypeInfo)
		require.NoError(t, err)
		require.NotNil(t, captured)
		assert.Equal(t, userID, captured.UserID)
		assert.Equal(t, "Title", captured.Title)
		assert.Equal(t, "Message", captured.Message)
		assert.Equal(t, domain.NotificationTypeInfo, captured.Type)
		assert.False(t, captured.IsRead)
		assert.NotEqual(t, uuid.Nil, captured.ID)
		assert.False(t, captured.CreatedAt.IsZero())
		assert.Equal(t, hub, svc.GetHub())
	})

	t.Run("repo error", func(t *testing.T) {
		hub := ws.NewHub()
		go hub.Run()
		defer hub.Stop()

		repo := notificationRepoMock{
			createFn:        func(context.Context, *domain.Notification) error { return errors.New("db error") },
			getByUserIDFn:   func(context.Context, uuid.UUID, int, int) ([]domain.Notification, error) { return nil, nil },
			markAsReadFn:    func(context.Context, uuid.UUID, uuid.UUID) error { return nil },
			markAllAsReadFn: func(context.Context, uuid.UUID) error { return nil },
			unreadCountFn:   func(context.Context, uuid.UUID) (int, error) { return 0, nil },
		}
		svc := NewNotificationService(repo, hub)

		err := svc.Create(context.Background(), uuid.New(), "t", "m", domain.NotificationTypeError)
		require.EqualError(t, err, "db error")
	})
}

func TestNotificationService_Delegates(t *testing.T) {
	userID := uuid.New()
	notificationID := uuid.New()
	expected := []domain.Notification{{ID: uuid.New(), UserID: userID, Title: "n"}}

	hub := ws.NewHub()
	repo := notificationRepoMock{
		createFn: func(context.Context, *domain.Notification) error { return nil },
		getByUserIDFn: func(_ context.Context, gotUserID uuid.UUID, limit, offset int) ([]domain.Notification, error) {
			assert.Equal(t, userID, gotUserID)
			assert.Equal(t, 10, limit)
			assert.Equal(t, 5, offset)
			return expected, nil
		},
		markAsReadFn: func(_ context.Context, gotNotificationID, gotUserID uuid.UUID) error {
			assert.Equal(t, notificationID, gotNotificationID)
			assert.Equal(t, userID, gotUserID)
			return nil
		},
		markAllAsReadFn: func(_ context.Context, gotUserID uuid.UUID) error {
			assert.Equal(t, userID, gotUserID)
			return nil
		},
		unreadCountFn: func(_ context.Context, gotUserID uuid.UUID) (int, error) {
			assert.Equal(t, userID, gotUserID)
			return 7, nil
		},
	}
	svc := NewNotificationService(repo, hub)
	ctx := context.Background()

	items, err := svc.GetUserNotifications(ctx, userID, 10, 5)
	require.NoError(t, err)
	assert.Equal(t, expected, items)

	require.NoError(t, svc.MarkAsRead(ctx, notificationID, userID))
	require.NoError(t, svc.MarkAllAsRead(ctx, userID))

	count, err := svc.UnreadCount(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, 7, count)
}
