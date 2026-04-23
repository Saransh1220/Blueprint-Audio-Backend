package email

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSender_Disabled(t *testing.T) {
	cfg := Config{Enabled: false}
	sender := NewSender(cfg)
	_, ok := sender.(noopSender)
	assert.True(t, ok)

	// test noop sender doesn't error
	err := sender.Send(context.Background(), Message{})
	assert.NoError(t, err)
}

func TestNewSender_MissingConfig(t *testing.T) {
	cfg := Config{Enabled: true, APIKey: "", From: "test@example.com"}
	sender := NewSender(cfg)
	_, ok := sender.(noopSender)
	assert.True(t, ok)
}

func TestNewSender_Enabled(t *testing.T) {
	cfg := Config{Enabled: true, APIKey: "key", From: "test@example.com", APIRoot: "http://api.local"}
	sender := NewSender(cfg)
	rs, ok := sender.(*resendSender)
	assert.True(t, ok)
	assert.Equal(t, "key", rs.apiKey)
	assert.Equal(t, "test@example.com", rs.from)
	assert.Equal(t, "http://api.local", rs.apiRoot)
	assert.NotNil(t, rs.httpClient)
}

func TestResendSender_Send_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/emails", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "test@example.com")
		assert.Contains(t, string(body), "recipient@example.com")
		assert.Contains(t, string(body), "Test Subject")
		assert.Contains(t, string(body), "html body")
		assert.Contains(t, string(body), "text body")
		assert.Contains(t, string(body), "reply@example.com")

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"mock-id"}`))
	}))
	defer ts.Close()

	cfg := Config{
		Enabled: true,
		APIKey:  "test-key",
		From:    "test@example.com",
		ReplyTo: "reply@example.com",
		APIRoot: ts.URL,
		HTTPClient: ts.Client(),
	}
	sender := NewSender(cfg)

	msg := Message{
		To:      []string{"recipient@example.com"},
		Subject: "Test Subject",
		HTML:    "html body",
		Text:    "text body",
	}

	err := sender.Send(context.Background(), msg)
	assert.NoError(t, err)
}

func TestResendSender_Send_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer ts.Close()

	cfg := Config{
		Enabled: true,
		APIKey:  "test-key",
		From:    "test@example.com",
		APIRoot: ts.URL,
		HTTPClient: ts.Client(),
	}
	sender := NewSender(cfg)

	err := sender.Send(context.Background(), Message{To: []string{"test@example.com"}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resend returned 400")
}

func TestResendSender_Send_NetworkError(t *testing.T) {
	// Create client with tight timeout
	client := &http.Client{Timeout: 1 * time.Millisecond}
	
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
	}))
	defer ts.Close()

	cfg := Config{
		Enabled: true,
		APIKey:  "test-key",
		From:    "test@example.com",
		APIRoot: ts.URL,
		HTTPClient: client,
	}
	sender := NewSender(cfg)

	err := sender.Send(context.Background(), Message{To: []string{"test@example.com"}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "send resend request")
}
