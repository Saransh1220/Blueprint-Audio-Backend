package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestLoggerMiddleware_PublicConstructorLogsToStdout(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	original := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = original })

	handler := RequestLoggerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	req := httptest.NewRequest(http.MethodPost, "/things", nil)
	req.RemoteAddr = "203.0.113.7:5555"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.NoError(t, writer.Close())
	os.Stdout = original
	out, err := io.ReadAll(reader)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Contains(t, string(out), "[API]")
	assert.Contains(t, string(out), "| 201 |")
	assert.Contains(t, string(out), "203.0.113.7")
	assert.Contains(t, string(out), `POST    "/things"`)
}

func TestRequestLoggerMiddleware_DefaultsToStatusOKWhenHandlerWritesNothing(t *testing.T) {
	var output bytes.Buffer
	clock := time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC)
	handler := newRequestLoggerMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		&output, false, func() time.Time { return clock })

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Contains(t, output.String(), "| 200 |")
}

func TestRequestLoggerMiddleware_FirstStatusWins(t *testing.T) {
	var output bytes.Buffer
	clock := time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC)
	rec := httptest.NewRecorder()
	handler := newRequestLoggerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		w.WriteHeader(http.StatusTeapot) // ignored by the wrapper
	}), &output, false, func() time.Time { return clock })

	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Contains(t, output.String(), "| 202 |")
	assert.NotContains(t, output.String(), "418")
}

func TestStatusAndMethodColor_Defaults(t *testing.T) {
	assert.Equal(t, ansiWhite, statusColor(http.StatusSwitchingProtocols))
	assert.Equal(t, ansiGreen, statusColor(http.StatusNoContent))
	assert.Equal(t, ansiBlue, methodColor(http.MethodOptions))
	assert.Equal(t, ansiBlue, methodColor(http.MethodHead))
	assert.Equal(t, ansiYellow, methodColor(http.MethodPut))
}

func TestFormatRequestLatency(t *testing.T) {
	assert.Equal(t, "1.234s", formatRequestLatency(1234567890*time.Nanosecond))
	assert.Equal(t, "1.23ms", formatRequestLatency(1234567*time.Nanosecond))
	assert.Equal(t, "123µs", formatRequestLatency(123456*time.Nanosecond))
	assert.Equal(t, "999ns", formatRequestLatency(999*time.Nanosecond))
}

func TestRequestRemoteIP(t *testing.T) {
	assert.Equal(t, "192.0.2.1", requestRemoteIP("192.0.2.1:80"))
	assert.Equal(t, "::1", requestRemoteIP("[::1]:8080"))
	assert.Equal(t, "no-port-here", requestRemoteIP("no-port-here"))
}

type flushPushRecorder struct {
	*httptest.ResponseRecorder
	pushed []string
}

func (f *flushPushRecorder) Push(target string, _ *http.PushOptions) error {
	f.pushed = append(f.pushed, target)
	return nil
}

func TestRequestLogResponseWriter_FlushPushUnwrap(t *testing.T) {
	t.Run("flush writes implicit header and flushes underlying writer", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := &requestLogResponseWriter{ResponseWriter: rec, status: http.StatusOK}
		rw.Flush()
		assert.True(t, rw.wroteHeader)
		assert.True(t, rec.Flushed)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("flush after header keeps explicit status", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := &requestLogResponseWriter{ResponseWriter: rec, status: http.StatusOK}
		rw.WriteHeader(http.StatusPartialContent)
		rw.Flush()
		assert.Equal(t, http.StatusPartialContent, rw.status)
		assert.True(t, rec.Flushed)
	})

	t.Run("push unsupported", func(t *testing.T) {
		rw := &requestLogResponseWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
		assert.ErrorIs(t, rw.Push("/style.css", nil), http.ErrNotSupported)
	})

	t.Run("push delegates to pusher", func(t *testing.T) {
		inner := &flushPushRecorder{ResponseRecorder: httptest.NewRecorder()}
		rw := &requestLogResponseWriter{ResponseWriter: inner, status: http.StatusOK}
		require.NoError(t, rw.Push("/app.js", nil))
		assert.Equal(t, []string{"/app.js"}, inner.pushed)
	})

	t.Run("unwrap returns underlying writer", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := &requestLogResponseWriter{ResponseWriter: rec}
		assert.Same(t, rec, rw.Unwrap())
	})
}
