package utils

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusBadRequest, "bad request", errors.New("invalid payload"))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &body)
	assert.NoError(t, err)
	assert.Equal(t, "bad request", body.Error)
	assert.Equal(t, "invalid payload", body.Details)

	w = httptest.NewRecorder()
	WriteError(w, http.StatusUnauthorized, "unauthorized", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	body = ErrorResponse{}
	err = json.Unmarshal(w.Body.Bytes(), &body)
	assert.NoError(t, err)
	assert.Equal(t, "unauthorized", body.Error)
	assert.Equal(t, "", body.Details)
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	payload := map[string]any{"ok": true}
	WriteJSON(w, http.StatusCreated, payload)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"ok":true}`, w.Body.String())
}
