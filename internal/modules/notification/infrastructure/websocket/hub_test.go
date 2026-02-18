package websocket

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHub_BroadcastAndUnicast(t *testing.T) {
	h := NewHub()
	go h.Run()
	defer h.Stop()

	userID := uuid.New()
	client := &Client{send: make(chan []byte, 2), userID: userID, hub: h}

	// Register via channel to avoid race
	h.register <- client
	// Wait for registration to process (simplified synchronization for test)
	time.Sleep(10 * time.Millisecond)

	h.BroadcastMessage([]byte("broadcast"))
	select {
	case msg := <-client.send:
		assert.Equal(t, "broadcast", string(msg))
	case <-time.After(2 * time.Second):
		t.Fatal("expected broadcast message")
	}

	h.SendToUser(userID, []byte("private"))
	select {
	case msg := <-client.send:
		assert.Equal(t, "private", string(msg))
	case <-time.After(2 * time.Second):
		t.Fatal("expected unicast message")
	}
}

func TestHub_SendToUser_OnlyMatchingClientsReceive(t *testing.T) {
	h := NewHub()
	go h.Run()
	defer h.Stop()

	targetID := uuid.New()
	otherID := uuid.New()

	target := &Client{send: make(chan []byte, 1), userID: targetID, hub: h}
	other := &Client{send: make(chan []byte, 1), userID: otherID, hub: h}

	h.register <- target
	h.register <- other
	time.Sleep(10 * time.Millisecond)

	h.SendToUser(targetID, []byte("only-target"))

	select {
	case msg := <-target.send:
		assert.Equal(t, "only-target", string(msg))
	case <-time.After(2 * time.Second):
		t.Fatal("target did not receive message")
	}

	select {
	case <-other.send:
		t.Fatal("non-target client should not receive unicast")
	default:
	}
}

func TestHub_SenderHelpers(t *testing.T) {
	h := NewHub()

	doneBroadcast := make(chan []byte, 1)
	go func() { doneBroadcast <- <-h.broadcast }()
	h.BroadcastMessage([]byte("x"))
	require.Equal(t, "x", string(<-doneBroadcast))

	doneUnicast := make(chan UnicastMessage, 1)
	go func() { doneUnicast <- <-h.unicast }()
	uid := uuid.New()
	h.SendToUser(uid, []byte("y"))
	got := <-doneUnicast
	require.Equal(t, uid, got.UserID)
	require.Equal(t, "y", string(got.Message))
}

func TestHub_UnregisterRemovesClientAndClosesChannel(t *testing.T) {
	h := NewHub()
	go h.Run()
	defer h.Stop()

	client := &Client{send: make(chan []byte, 1), userID: uuid.New(), hub: h}
	h.register <- client
	time.Sleep(10 * time.Millisecond)

	// We can't safely access h.clients here without a mutex or similar mechanism on Hub
	// But we can test the *effect* of unregistering: the channel should be closed.

	h.unregister <- client
	time.Sleep(10 * time.Millisecond)

	// Verify channel is closed
	_, ok := <-client.send
	assert.False(t, ok, "client send channel should be closed")
}

func TestHub_DropsBlockedClientOnBroadcastAndUnicast(t *testing.T) {
	t.Run("broadcast blocked send removes client", func(t *testing.T) {
		h := NewHub()
		go h.Run()
		defer h.Stop()

		// Unbuffered channel without receiver forces default branch.
		client := &Client{send: make(chan []byte), userID: uuid.New(), hub: h}
		h.register <- client
		time.Sleep(10 * time.Millisecond)

		h.BroadcastMessage([]byte("x"))
		time.Sleep(10 * time.Millisecond)

		// Verify channel is closed (client removed)
		_, ok := <-client.send
		assert.False(t, ok, "blocked client should be removed and channel closed")
	})

	t.Run("unicast blocked send removes client", func(t *testing.T) {
		h := NewHub()
		go h.Run()
		defer h.Stop()

		uid := uuid.New()
		client := &Client{send: make(chan []byte), userID: uid, hub: h}
		h.register <- client
		time.Sleep(10 * time.Millisecond)

		h.SendToUser(uid, []byte("x"))
		time.Sleep(10 * time.Millisecond)

		// Verify channel is closed (client removed)
		_, ok := <-client.send
		assert.False(t, ok, "blocked client should be removed and channel closed")
	})
}
