package middleware

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiBlue   = "\x1b[34m"
	ansiCyan   = "\x1b[36m"
	ansiWhite  = "\x1b[37m"
)

// RequestLoggerMiddleware writes one compact access-log line after every request.
// It uses only the standard library and enables colors outside production unless
// NO_COLOR is set.
func RequestLoggerMiddleware(next http.Handler) http.Handler {
	return newRequestLoggerMiddleware(next, os.Stdout, shouldColorizeRequestLogs(), time.Now)
}

func newRequestLoggerMiddleware(
	next http.Handler,
	out io.Writer,
	color bool,
	now func() time.Time,
) http.Handler {
	logger := log.New(out, "", 0)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := now()
		rw := &requestLogResponseWriter{ResponseWriter: w, status: http.StatusOK}

		defer func() {
			status := fmt.Sprintf("%3d", rw.status)
			method := fmt.Sprintf("%-7s", r.Method)
			if color {
				status = statusColor(rw.status) + status + ansiReset
				method = methodColor(r.Method) + method + ansiReset
			}

			logger.Printf(
				"[API] %s | %s | %10s | %-15s | %s %s",
				start.Format("2006/01/02 - 15:04:05"),
				status,
				formatRequestLatency(now().Sub(start)),
				requestRemoteIP(r.RemoteAddr),
				method,
				safeRequestTarget(r),
			)
		}()

		next.ServeHTTP(rw, r)
	})
}

func safeRequestTarget(r *http.Request) string {
	requestURL := *r.URL
	query := requestURL.Query()
	for key := range query {
		if isSensitiveQueryKey(key) {
			query.Set(key, "[REDACTED]")
		}
	}
	requestURL.RawQuery = query.Encode()
	return strconv.Quote(requestURL.RequestURI())
}

func isSensitiveQueryKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	switch normalized {
	case "token", "access_token", "refresh_token", "id_token", "api_key", "authorization",
		"signature", "credential", "x_amz_signature", "x_amz_credential", "x_amz_security_token":
		return true
	default:
		return false
	}
}

func shouldColorizeRequestLogs() bool {
	return os.Getenv("NO_COLOR") == "" && !strings.EqualFold(os.Getenv("ENV"), "production")
}

func statusColor(status int) string {
	switch {
	case status >= 500:
		return ansiRed
	case status >= 400:
		return ansiYellow
	case status >= 300:
		return ansiCyan
	case status >= 200:
		return ansiGreen
	default:
		return ansiWhite
	}
}

func methodColor(method string) string {
	switch method {
	case http.MethodGet:
		return ansiCyan
	case http.MethodPost:
		return ansiGreen
	case http.MethodPut, http.MethodPatch:
		return ansiYellow
	case http.MethodDelete:
		return ansiRed
	default:
		return ansiBlue
	}
}

func formatRequestLatency(duration time.Duration) string {
	switch {
	case duration >= time.Second:
		return duration.Truncate(time.Millisecond).String()
	case duration >= time.Millisecond:
		return duration.Truncate(10 * time.Microsecond).String()
	case duration >= time.Microsecond:
		return duration.Truncate(time.Microsecond).String()
	default:
		return duration.String()
	}
}

func requestRemoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}

type requestLogResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rw *requestLogResponseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.status = code
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *requestLogResponseWriter) Write(body []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(body)
}

// Hijack preserves WebSocket upgrades through the logging middleware.
func (rw *requestLogResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijack not supported")
	}

	conn, buffer, err := hijacker.Hijack()
	if err == nil {
		rw.status = http.StatusSwitchingProtocols
		rw.wroteHeader = true
	}
	return conn, buffer, err
}

// Flush preserves streaming responses such as server-sent events.
func (rw *requestLogResponseWriter) Flush() {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	_ = http.NewResponseController(rw.ResponseWriter).Flush()
}

// Push preserves HTTP/2 server push support when provided by the server.
func (rw *requestLogResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := rw.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

func (rw *requestLogResponseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
