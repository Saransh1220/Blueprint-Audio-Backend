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

func TestUpgrader_CheckOrigin(t *testing.T) {
	cases := []struct {
		name   string
		origin string
		want   bool
	}{
		{name: "empty origin allowed", origin: "", want: true},
		{name: "localhost4200 allowed", origin: "http://localhost:4200", want: true},
		{name: "localhost3000 allowed", origin: "http://localhost:3000", want: true},
		{name: "production allowed", origin: "https://redwave.app", want: true},
		{name: "other denied", origin: "https://evil.example", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ws", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			got := upgrader.CheckOrigin(req)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestServeWs_EndToEndUnicast(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	userID := uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r, userID)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Give hub/register goroutine a moment to attach client before unicast.
	time.Sleep(100 * time.Millisecond)
	hub.SendToUser(userID, []byte("notify"))
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
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

func TestClient_WritePump_ClosedChannelSendsCloseFrame(t *testing.T) {
	hub := &Hub{
		register:   make(chan *Client, 1),
		unregister: make(chan *Client, 1),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r, uuid.New())
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	registered := <-hub.register
	close(registered.send)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage()
	assert.Error(t, err)
}
