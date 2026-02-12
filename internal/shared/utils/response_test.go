package utils

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusBadRequest, "bad request", errors.New("details"))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "\"error\":\"bad request\"")
	assert.Contains(t, w.Body.String(), "\"details\":\"details\"")
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusCreated, map[string]string{"ok": "true"})
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), "\"ok\":\"true\"")
}

func TestIsValidEmail(t *testing.T) {
	assert.True(t, IsValidEmail("a@b.com"))
	assert.False(t, IsValidEmail("invalid"))
}

