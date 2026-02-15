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
	userID := uuid.New()
	client := &Client{send: make(chan []byte, 2), userID: userID}
	h.clients[client] = true

	go h.Run()

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
	targetID := uuid.New()
	otherID := uuid.New()

	target := &Client{send: make(chan []byte, 1), userID: targetID}
	other := &Client{send: make(chan []byte, 1), userID: otherID}
	h.clients[target] = true
	h.clients[other] = true

	go h.Run()

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
