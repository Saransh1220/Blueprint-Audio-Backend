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

// dialTestClient starts ServeWs against a hub whose channels are buffered so the
// test can observe register/unregister events directly.
func dialTestClient(t *testing.T) (*Hub, *Client, *websocket.Conn) {
	t.Helper()
	hub := &Hub{
		register:   make(chan *Client, 1),
		unregister: make(chan *Client, 1),
		stop:       make(chan struct{}),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r, uuid.New())
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(hub.stop) })

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	select {
	case registered := <-hub.register:
		return hub, registered, conn
	case <-time.After(2 * time.Second):
		t.Fatal("client never registered")
		return nil, nil, nil
	}
}

func expectUnregister(t *testing.T, hub *Hub, want *Client) {
	t.Helper()
	select {
	case got := <-hub.unregister:
		assert.Same(t, want, got)
	case <-time.After(2 * time.Second):
		t.Fatal("client was not unregistered")
	}
}

func TestClient_ReadPump_UnexpectedCloseCodeUnregisters(t *testing.T) {
	hub, client, conn := dialTestClient(t)

	require.NoError(t, conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "bye"),
		time.Now().Add(time.Second),
	))

	expectUnregister(t, hub, client)
}

func TestClient_ReadPump_PongAndIgnoredMessagesKeepConnectionOpen(t *testing.T) {
	hub, client, conn := dialTestClient(t)

	require.NoError(t, conn.WriteControl(websocket.PongMessage, nil, time.Now().Add(time.Second)))
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("ignored")))

	select {
	case <-hub.unregister:
		t.Fatal("client must stay registered after pong and ordinary messages")
	case <-time.After(200 * time.Millisecond):
	}

	// Still alive: a hub push reaches the peer.
	client.send <- []byte("still-here")
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, body, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "still-here", string(body))
}

func TestClient_ReadPump_OversizedMessageDropsClient(t *testing.T) {
	hub, client, conn := dialTestClient(t)

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", maxMessageSize+1))))

	expectUnregister(t, hub, client)
}

func TestClient_ReadPump_ExitsWithoutBlockingWhenHubStopped(t *testing.T) {
	hub := &Hub{
		register:   make(chan *Client, 1),
		unregister: make(chan *Client), // unbuffered and never read
		stop:       make(chan struct{}),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r, uuid.New())
	}))
	defer srv.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()
	client := <-hub.register

	close(hub.stop)
	require.NoError(t, conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second),
	))

	// readPump must release via hub.stop; the server then drops the socket, so
	// the peer observes a read error instead of hanging forever.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage()
	assert.Error(t, err)
	assert.NotNil(t, client)
}
