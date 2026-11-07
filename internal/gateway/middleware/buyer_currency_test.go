package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
	"github.com/stretchr/testify/assert"
)

func TestBuyerCurrencyMiddleware(t *testing.T) {
	testUserID := uuid.New()

	lookup := func(ctx context.Context, id uuid.UUID) (*string, error) {
		if id == testUserID {
			curr := "INR"
			return &curr, nil
		}
		return nil, nil
	}

	mw := NewBuyerCurrencyMiddleware(lookup, true)

	t.Run("authenticated user profile preference", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/specs", nil)
		ctx := context.WithValue(req.Context(), ContextKeyUserId, testUserID)
		req = req.WithContext(ctx)
		req.Header.Set("CF-IPCountry", "US")

		var capturedCurrency string
		var capturedSource money.CurrencySource

		handler := mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCurrency = BuyerCurrencyFromContext(r.Context())
			capturedSource = BuyerResolutionFromContext(r.Context()).Source
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, "INR", capturedCurrency)
		assert.Equal(t, money.SourceProfile, capturedSource)
		assert.Contains(t, rec.Header().Values("Vary"), "Cookie")
		assert.Contains(t, rec.Header().Values("Vary"), "CF-IPCountry")
		assert.Contains(t, rec.Header().Values("Vary"), "X-Country-Code")
	})

	t.Run("cookie preference for guest", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/specs", nil)
		req.AddCookie(&http.Cookie{Name: "wy_currency", Value: "INR"})
		req.Header.Set("CF-IPCountry", "US")

		var capturedCurrency string
		var capturedSource money.CurrencySource

		handler := mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCurrency = BuyerCurrencyFromContext(r.Context())
			capturedSource = BuyerResolutionFromContext(r.Context()).Source
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, "INR", capturedCurrency)
		assert.Equal(t, money.SourceCookie, capturedSource)
	})

	t.Run("country header fallback", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/specs", nil)
		req.Header.Set("CF-IPCountry", "IN")

		var capturedCurrency string
		var capturedSource money.CurrencySource

		handler := mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCurrency = BuyerCurrencyFromContext(r.Context())
			capturedSource = BuyerResolutionFromContext(r.Context()).Source
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, "INR", capturedCurrency)
		assert.Equal(t, money.SourceCountry, capturedSource)
	})

	t.Run("default fallback when headers empty", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/specs", nil)

		var capturedCurrency string
		var capturedSource money.CurrencySource

		handler := mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCurrency = BuyerCurrencyFromContext(r.Context())
			capturedSource = BuyerResolutionFromContext(r.Context()).Source
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, "USD", capturedCurrency)
		assert.Equal(t, money.SourceDefault, capturedSource)
	})

	t.Run("context helpers with nil or empty context", func(t *testing.T) {
		assert.Equal(t, "USD", BuyerCurrencyFromContext(nil))
		assert.Equal(t, "USD", BuyerCurrencyFromContext(context.Background()))

		resNil := BuyerResolutionFromContext(nil)
		assert.Equal(t, "USD", resNil.Currency)
		assert.Equal(t, money.SourceDefault, resNil.Source)

		resBg := BuyerResolutionFromContext(context.Background())
		assert.Equal(t, "USD", resBg.Currency)
		assert.Equal(t, money.SourceDefault, resBg.Source)
	})
}
