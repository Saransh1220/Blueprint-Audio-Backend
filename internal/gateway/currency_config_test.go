package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saransh1220/blueprint-audio/internal/shared/money"
	"github.com/stretchr/testify/require"
)

func TestCurrencyConfiguration(t *testing.T) {
	names := map[string]string{"INR": "razorpay", "USD": "dodo"}
	handler := currencyConfiguration(names)
	names["USD"] = "changed-after-startup"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/config/currencies", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	require.Equal(t, "public, max-age=300", rr.Header().Get("Cache-Control"))
	var body struct {
		Currencies []money.CurrencyDefinition `json:"currencies"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	require.Len(t, body.Currencies, 2)
	require.Equal(t, "INR", body.Currencies[0].Code)
	require.Equal(t, "₹", body.Currencies[0].Symbol)
	require.Equal(t, 100, body.Currencies[0].MinorUnitScale)
	require.Equal(t, "en-IN", body.Currencies[0].Locale)
	require.Equal(t, "razorpay", *body.Currencies[0].Gateway)
	require.Equal(t, "dodo", *body.Currencies[1].Gateway)

	rr = httptest.NewRecorder()
	currencyConfiguration(nil).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/config/currencies", nil))
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	require.Nil(t, body.Currencies[0].Gateway)
	require.Nil(t, body.Currencies[1].Gateway)
}

func TestCurrencyConfiguration_ContractAndEdgeCases(t *testing.T) {
	t.Run("strict contract shape and headers", func(t *testing.T) {
		registry := map[string]string{
			"INR": "razorpay",
			"USD": "dodo",
		}
		handler := currencyConfiguration(registry)

		req := httptest.NewRequest(http.MethodGet, "/config/currencies", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
		require.Equal(t, "public, max-age=300", rec.Header().Get("Cache-Control"))

		var raw map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
		require.Contains(t, raw, "currencies")

		items, ok := raw["currencies"].([]any)
		require.True(t, ok)
		supported := money.SupportedCurrencies()
		require.Equal(t, len(supported), len(items))

		for i, rawItem := range items {
			itemMap, ok := rawItem.(map[string]any)
			require.True(t, ok)
			require.Equal(t, supported[i].Code, itemMap["code"])
			require.Equal(t, supported[i].Symbol, itemMap["symbol"])
			require.Equal(t, float64(supported[i].MinorUnitScale), itemMap["minor_unit_scale"])
			require.Equal(t, supported[i].Locale, itemMap["locale"])
			require.Contains(t, itemMap, "gateway")
			require.NotNil(t, itemMap["gateway"])
		}
	})

	t.Run("null gateway when unrouted", func(t *testing.T) {
		handler := currencyConfiguration(map[string]string{
			"INR": "razorpay",
		})

		req := httptest.NewRequest(http.MethodGet, "/config/currencies", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)

		var raw map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
		items := raw["currencies"].([]any)

		// USD should have null gateway
		for _, rawItem := range items {
			itemMap := rawItem.(map[string]any)
			if itemMap["code"] == "USD" {
				require.Nil(t, itemMap["gateway"])
			} else if itemMap["code"] == "INR" {
				require.Equal(t, "razorpay", itemMap["gateway"])
			}
		}
	})
}
