package frankfurter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/saransh1220/blueprint-audio/internal/modules/fx/infrastructure/provider/frankfurter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider_Validation(t *testing.T) {
	// Remote HTTP blocked
	_, err := frankfurter.NewProvider(frankfurter.Config{
		BaseURL: "http://example.com/api",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must use HTTPS")

	// Local HTTP allowed
	pLocal, err := frankfurter.NewProvider(frankfurter.Config{
		BaseURL: "http://localhost:8080/api",
	})
	require.NoError(t, err)
	assert.Equal(t, "frankfurter", pLocal.Name())

	// Default URL when empty
	pDefault, err := frankfurter.NewProvider(frankfurter.Config{})
	require.NoError(t, err)
	assert.NotNil(t, pDefault)
}

func TestProvider_Fetch_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/latest", r.URL.Path)
		assert.Equal(t, "USD", r.URL.Query().Get("base"))
		assert.Equal(t, "INR", r.URL.Query().Get("symbols"))
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"amount": 1.0,
			"base": "USD",
			"date": "2026-11-06",
			"rates": {
				"INR": 83.12345678
			}
		}`))
	}))
	defer server.Close()

	p, err := frankfurter.NewProvider(frankfurter.Config{
		BaseURL:    server.URL + "/v1/latest",
		APIKey:     "test-key",
		HTTPClient: server.Client(),
	})
	require.NoError(t, err)

	rates, err := p.Fetch(context.Background(), "USD", []string{"INR"})
	require.NoError(t, err)
	assert.Equal(t, "USD", rates.Base)
	assert.Equal(t, "frankfurter", rates.Source)
	assert.Equal(t, "2026-11-06", rates.AsOf.Format(time.DateOnly))

	inrRate, exists := rates.Rates["INR"]
	require.True(t, exists)
	assert.Equal(t, "4156172839/50000000", inrRate.String())
}

func TestProvider_Fetch_Errors(t *testing.T) {
	t.Run("invalid currency codes", func(t *testing.T) {
		p, _ := frankfurter.NewProvider(frankfurter.Config{BaseURL: "https://localhost/api"})
		_, err := p.Fetch(context.Background(), "INVALID", []string{"INR"})
		assert.Error(t, err)

		_, err = p.Fetch(context.Background(), "USD", []string{"IN"})
		assert.Error(t, err)

		_, err = p.Fetch(context.Background(), "USD", []string{"USD"}) // only self requested
		assert.Error(t, err)
	})

	t.Run("provider 500 error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		p, err := frankfurter.NewProvider(frankfurter.Config{
			BaseURL:    server.URL,
			HTTPClient: server.Client(),
		})
		require.NoError(t, err)

		_, err = p.Fetch(context.Background(), "USD", []string{"INR"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "status: 500")
	})

	t.Run("mismatched base currency", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"amount":1,"base":"EUR","date":"2026-11-06","rates":{"INR":90}}`))
		}))
		defer server.Close()

		p, _ := frankfurter.NewProvider(frankfurter.Config{
			BaseURL:    server.URL,
			HTTPClient: server.Client(),
		})

		_, err := p.Fetch(context.Background(), "USD", []string{"INR"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mismatched base currency")
	})

	t.Run("invalid rate values", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"amount":1,"base":"USD","date":"2026-11-06","rates":{"INR":-10}}`))
		}))
		defer server.Close()

		p, _ := frankfurter.NewProvider(frankfurter.Config{
			BaseURL:    server.URL,
			HTTPClient: server.Client(),
		})

		_, err := p.Fetch(context.Background(), "USD", []string{"INR"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "non-positive")
	})

	t.Run("invalid date", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"amount":1,"base":"USD","date":"invalid-date","rates":{"INR":83}}`))
		}))
		defer server.Close()

		p, _ := frankfurter.NewProvider(frankfurter.Config{
			BaseURL:    server.URL,
			HTTPClient: server.Client(),
		})

		_, err := p.Fetch(context.Background(), "USD", []string{"INR"})
		assert.Error(t, err)
	})

	t.Run("payload size cap (over 1MB)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			// Send huge padded whitespace
			_, _ = w.Write([]byte(`{"amount":1,"base":"USD","date":"2026-11-06","rates":{"INR":83}` + strings.Repeat(" ", 1<<20+100) + `}`))
		}))
		defer server.Close()

		p, _ := frankfurter.NewProvider(frankfurter.Config{
			BaseURL:    server.URL,
			HTTPClient: server.Client(),
		})

		_, err := p.Fetch(context.Background(), "USD", []string{"INR"})
		assert.Error(t, err)
	})
}
