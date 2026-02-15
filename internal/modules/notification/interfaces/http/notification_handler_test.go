package http_test

import (
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/notification/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/notification/domain"
	ws "github.com/saransh1220/blueprint-audio/internal/modules/notification/infrastructure/websocket"
	notificationhttp "github.com/saransh1220/blueprint-audio/internal/modules/notification/interfaces/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type notificationRepoStub struct {
	getByUserIDFn   func(context.Context, uuid.UUID, int, int) ([]domain.Notification, error)
	markAsReadFn    func(context.Context, uuid.UUID, uuid.UUID) error
	markAllAsReadFn func(context.Context, uuid.UUID) error
	unreadCountFn   func(context.Context, uuid.UUID) (int, error)
}

func (s notificationRepoStub) Create(context.Context, *domain.Notification) error { return nil }
func (s notificationRepoStub) GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.Notification, error) {
	return s.getByUserIDFn(ctx, userID, limit, offset)
}
func (s notificationRepoStub) MarkAsRead(ctx context.Context, notificationID, userID uuid.UUID) error {
	return s.markAsReadFn(ctx, notificationID, userID)
}
func (s notificationRepoStub) MarkAllAsRead(ctx context.Context, userID uuid.UUID) error {
	return s.markAllAsReadFn(ctx, userID)
}
func (s notificationRepoStub) UnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	return s.unreadCountFn(ctx, userID)
}

func authedRequest(method, path string, userID uuid.UUID) *stdhttp.Request {
	req := httptest.NewRequest(method, path, nil)
	ctx := context.WithValue(req.Context(), middleware.ContextKeyUserId, userID)
	return req.WithContext(ctx)
}

func newHandler(repo notificationRepoStub, hub *ws.Hub) *notificationhttp.NotificationHandler {
	svc := application.NewNotificationService(repo, hub)
	return notificationhttp.NewNotificationHandler(svc, hub)
}

func TestNotificationHandler_SubscribeAndList(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()
	defer hub.Stop()

	userID := uuid.New()
	h := newHandler(notificationRepoStub{
		getByUserIDFn: func(_ context.Context, gotUserID uuid.UUID, limit, offset int) ([]domain.Notification, error) {
			assert.Equal(t, userID, gotUserID)
			assert.Equal(t, 5, limit)
			assert.Equal(t, 2, offset)
			return []domain.Notification{{ID: uuid.New(), UserID: userID, Title: "A"}}, nil
		},
		markAsReadFn:    func(context.Context, uuid.UUID, uuid.UUID) error { return nil },
		markAllAsReadFn: func(context.Context, uuid.UUID) error { return nil },
		unreadCountFn:   func(context.Context, uuid.UUID) (int, error) { return 0, nil },
	}, hub)

	w := httptest.NewRecorder()
	h.Subscribe(w, httptest.NewRequest(stdhttp.MethodGet, "/notifications/subscribe", nil))
	assert.Equal(t, stdhttp.StatusUnauthorized, w.Code)

	w = httptest.NewRecorder()
	req := authedRequest(stdhttp.MethodGet, "/notifications?limit=5&offset=2", userID)
	h.ListNotifications(w, req)
	assert.Equal(t, stdhttp.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"data"`)
}

func TestNotificationHandler_ErrorAndMutationBranches(t *testing.T) {
	userID := uuid.New()
	nID := uuid.New()
	hub := ws.NewHub()
	go hub.Run()
	defer hub.Stop()

	h := newHandler(notificationRepoStub{
		getByUserIDFn: func(context.Context, uuid.UUID, int, int) ([]domain.Notification, error) {
			return nil, errors.New("db")
		},
		markAsReadFn:    func(context.Context, uuid.UUID, uuid.UUID) error { return errors.New("db") },
		markAllAsReadFn: func(context.Context, uuid.UUID) error { return errors.New("db") },
		unreadCountFn:   func(context.Context, uuid.UUID) (int, error) { return 0, errors.New("db") },
	}, hub)

	w := httptest.NewRecorder()
	h.ListNotifications(w, httptest.NewRequest(stdhttp.MethodGet, "/notifications", nil))
	assert.Equal(t, stdhttp.StatusUnauthorized, w.Code)

	w = httptest.NewRecorder()
	h.ListNotifications(w, authedRequest(stdhttp.MethodGet, "/notifications", userID))
	assert.Equal(t, stdhttp.StatusInternalServerError, w.Code)

	w = httptest.NewRecorder()
	badReq := httptest.NewRequest(stdhttp.MethodPatch, "/notifications/read/bad", nil)
	badReq.SetPathValue("id", "bad")
	h.MarkAsRead(w, badReq)
	assert.Equal(t, stdhttp.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	req := authedRequest(stdhttp.MethodPatch, "/notifications/read/"+nID.String(), userID)
	req.SetPathValue("id", nID.String())
	h.MarkAsRead(w, req)
	assert.Equal(t, stdhttp.StatusInternalServerError, w.Code)

	w = httptest.NewRecorder()
	h.MarkAllAsRead(w, httptest.NewRequest(stdhttp.MethodPatch, "/notifications/read-all", nil))
	assert.Equal(t, stdhttp.StatusUnauthorized, w.Code)

	w = httptest.NewRecorder()
	h.MarkAllAsRead(w, authedRequest(stdhttp.MethodPatch, "/notifications/read-all", userID))
	assert.Equal(t, stdhttp.StatusInternalServerError, w.Code)

	w = httptest.NewRecorder()
	h.UnreadCount(w, httptest.NewRequest(stdhttp.MethodGet, "/notifications/unread-count", nil))
	assert.Equal(t, stdhttp.StatusUnauthorized, w.Code)

	w = httptest.NewRecorder()
	h.UnreadCount(w, authedRequest(stdhttp.MethodGet, "/notifications/unread-count", userID))
	assert.Equal(t, stdhttp.StatusInternalServerError, w.Code)
}

func TestNotificationHandler_SuccessBranches(t *testing.T) {
	userID := uuid.New()
	nID := uuid.New()
	hub := ws.NewHub()
	go hub.Run()
	defer hub.Stop()

	h := newHandler(notificationRepoStub{
		getByUserIDFn:   func(context.Context, uuid.UUID, int, int) ([]domain.Notification, error) { return nil, nil },
		markAsReadFn:    func(context.Context, uuid.UUID, uuid.UUID) error { return nil },
		markAllAsReadFn: func(context.Context, uuid.UUID) error { return nil },
		unreadCountFn:   func(context.Context, uuid.UUID) (int, error) { return 3, nil },
	}, hub)

	w := httptest.NewRecorder()
	req := authedRequest(stdhttp.MethodPatch, "/notifications/read/"+nID.String(), userID)
	req.SetPathValue("id", nID.String())
	h.MarkAsRead(w, req)
	assert.Equal(t, stdhttp.StatusNoContent, w.Code)

	w = httptest.NewRecorder()
	h.MarkAllAsRead(w, authedRequest(stdhttp.MethodPatch, "/notifications/read-all", userID))
	assert.Equal(t, stdhttp.StatusNoContent, w.Code)

	w = httptest.NewRecorder()
	h.UnreadCount(w, authedRequest(stdhttp.MethodGet, "/notifications/unread-count", userID))
	assert.Equal(t, stdhttp.StatusOK, w.Code)

	var payload map[string]int
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, 3, payload["count"])
}
