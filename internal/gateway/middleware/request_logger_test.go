package middleware

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestLoggerMiddleware_FormatsRequestWithoutColor(t *testing.T) {
	var output bytes.Buffer
	times := []time.Time{
		time.Date(2026, time.July, 20, 11, 22, 33, 0, time.UTC),
		time.Date(2026, time.July, 20, 11, 22, 33, int(1250*time.Microsecond), time.UTC),
	}
	now := func() time.Time {
		current := times[0]
		times = times[1:]
		return current
	}

	handler := newRequestLoggerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}), &output, false, now)

	req := httptest.NewRequest(http.MethodGet, "/missing?q=beat", nil)
	req.RemoteAddr = "192.0.2.10:4321"
	handler.ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, "[API] 2026/07/20 - 11:22:33 | 404 |     1.25ms | 192.0.2.10      | GET     \"/missing?q=beat\"\n", output.String())
}

func TestRequestLoggerMiddleware_ColorsStatusesAndMethods(t *testing.T) {
	tests := []struct {
		status      int
		method      string
		statusColor string
		methodColor string
	}{
		{http.StatusOK, http.MethodGet, ansiGreen, ansiCyan},
		{http.StatusFound, http.MethodPost, ansiCyan, ansiGreen},
		{http.StatusBadRequest, http.MethodPatch, ansiYellow, ansiYellow},
		{http.StatusInternalServerError, http.MethodDelete, ansiRed, ansiRed},
	}

	for _, test := range tests {
		t.Run(test.method, func(t *testing.T) {
			var output bytes.Buffer
			clock := time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC)
			handler := newRequestLoggerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
			}), &output, true, func() time.Time { return clock })

			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(test.method, "/", nil))

			assert.Contains(t, output.String(), test.statusColor+fmt.Sprintf("%3d", test.status)+ansiReset)
			assert.Contains(t, output.String(), test.methodColor+fmt.Sprintf("%-7s", test.method)+ansiReset)
		})
	}
}

func TestRequestLoggerMiddleware_ColorConfiguration(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("NO_COLOR", "")
	assert.False(t, shouldColorizeRequestLogs())

	t.Setenv("ENV", "development")
	assert.True(t, shouldColorizeRequestLogs())

	t.Setenv("NO_COLOR", "1")
	assert.False(t, shouldColorizeRequestLogs())
}

func TestRequestLoggerMiddleware_RedactsSensitiveQueryValues(t *testing.T) {
	var output bytes.Buffer
	clock := time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC)
	handler := newRequestLoggerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	}), &output, false, func() time.Time { return clock })

	req := httptest.NewRequest(http.MethodGet, "/ws?token=secret-jwt&topic=orders", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	assert.NotContains(t, output.String(), "secret-jwt")
	assert.Contains(t, output.String(), "token=%5BREDACTED%5D")
	assert.Contains(t, output.String(), "topic=orders")
}

func TestRequestLogResponseWriter_PreservesImplicitStatusAndHijacking(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := &requestLogResponseWriter{ResponseWriter: recorder, status: http.StatusOK}

	_, err := rw.Write([]byte("ok"))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rw.status)
	assert.True(t, rw.wroteHeader)

	base := httptest.NewRecorder()
	hijackWriter := &requestLogResponseWriter{
		ResponseWriter: requestLoggerHijackableWriter{ResponseWriter: base},
		status:         http.StatusOK,
	}
	conn, _, err := hijackWriter.Hijack()
	require.NoError(t, err)
	assert.Equal(t, http.StatusSwitchingProtocols, hijackWriter.status)
	require.NotNil(t, conn)
	_ = conn.Close()
}

func TestRequestLogResponseWriter_RejectsUnsupportedHijacking(t *testing.T) {
	rw := &requestLogResponseWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	_, _, err := rw.Hijack()
	assert.EqualError(t, err, "hijack not supported")
}

type requestLoggerHijackableWriter struct {
	http.ResponseWriter
}

func (w requestLoggerHijackableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	server, client := net.Pipe()
	_ = client.Close()
	return server, bufio.NewReadWriter(bufio.NewReader(server), bufio.NewWriter(server)), nil
}
