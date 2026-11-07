package utils

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBuildAndSetCookie(t *testing.T) {
	t.Run("insecure cookie has Strict SameSite", func(t *testing.T) {
		cookie := BuildCookie("test_name", "test_val", "/", time.Now().Add(time.Hour), 3600, false, true)
		assert.Equal(t, "test_name", cookie.Name)
		assert.Equal(t, "test_val", cookie.Value)
		assert.False(t, cookie.Secure)
		assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
		assert.True(t, cookie.HttpOnly)
	})

	t.Run("secure cookie has None SameSite", func(t *testing.T) {
		cookie := BuildCookie("test_secure", "val", "/", time.Now().Add(time.Hour), 3600, true, true)
		assert.True(t, cookie.Secure)
		assert.Equal(t, http.SameSiteNoneMode, cookie.SameSite)
	})

	t.Run("SetCookie sets header on ResponseWriter", func(t *testing.T) {
		rec := httptest.NewRecorder()
		SetCookie(rec, "wy_currency", "INR", "/", time.Now().Add(24*time.Hour), 86400, false, true)

		cookies := rec.Result().Cookies()
		assert.Len(t, cookies, 1)
		assert.Equal(t, "wy_currency", cookies[0].Name)
		assert.Equal(t, "INR", cookies[0].Value)
	})
}
