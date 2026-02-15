package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeWs_EndToEndUnicast(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	userID := uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r, userID)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("hello")))

	hub.SendToUser(userID, []byte("notify"))
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	msgType, body, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, msgType)
	assert.Equal(t, "notify", string(body))
}

func TestServeWs_UpgradeFailure(t *testing.T) {
	hub := NewHub()
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	w := httptest.NewRecorder()

	ServeWs(hub, w, req, uuid.New())

	// Upgrade fails for normal HTTP request and upgrader writes bad request.
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
