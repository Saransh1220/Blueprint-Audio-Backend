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

func TestClient_WritePump_DrainsQueuedMessages(t *testing.T) {
	hub := &Hub{
		register:   make(chan *Client, 1),
		unregister: make(chan *Client, 1),
		stop:       make(chan struct{}),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r, uuid.New())
	}))
	defer srv.Close()
	defer close(hub.stop)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	registered := <-hub.register
	registered.send <- []byte("one")
	registered.send <- []byte("two")

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg1, err := conn.ReadMessage()
	require.NoError(t, err)
	_, msg2, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "one", string(msg1))
	assert.Equal(t, "two", string(msg2))
}

func TestServeWs_WhenHubStopped_DoesNotBlockRegistration(t *testing.T) {
	hub := &Hub{
		register:   make(chan *Client),
		unregister: make(chan *Client),
		stop:       make(chan struct{}),
	}
	close(hub.stop)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r, uuid.New())
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	_ = conn.Close()
}

func TestClient_WritePump_StopsWhenConnectionClosed(t *testing.T) {
	hub := &Hub{
		register:   make(chan *Client, 1),
		unregister: make(chan *Client, 1),
		stop:       make(chan struct{}),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r, uuid.New())
	}))
	defer srv.Close()
	defer close(hub.stop)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	registered := <-hub.register
	require.NoError(t, conn.Close())

	// writePump should hit a write error path and return without blocking.
	registered.send <- []byte("after-close")
	time.Sleep(50 * time.Millisecond)
}
