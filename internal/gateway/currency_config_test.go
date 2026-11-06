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
