package utils

import (
	"net/http"
	"time"
)

// BuildCookie creates an http.Cookie configuring SameSite and Secure consistently.
// If secure is true, SameSite is None; otherwise Strict.
func BuildCookie(name, value, path string, expires time.Time, maxAge int, secure bool, httpOnly bool) *http.Cookie {
	sameSite := http.SameSiteStrictMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}

	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		Expires:  expires,
		MaxAge:   maxAge,
		HttpOnly: httpOnly,
		Secure:   secure,
		SameSite: sameSite,
	}
}

// SetCookie configures and sets an HTTP cookie on the response writer using BuildCookie.
func SetCookie(w http.ResponseWriter, name, value, path string, expires time.Time, maxAge int, secure bool, httpOnly bool) {
	http.SetCookie(w, BuildCookie(name, value, path, expires, maxAge, secure, httpOnly))
}
