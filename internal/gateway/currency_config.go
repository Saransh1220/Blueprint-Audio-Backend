package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

// currencyConfiguration snapshots the configured gateway names, never credentials.
// An absent gateway is represented by null rather than an invented provider.
func currencyConfiguration(gatewayNames map[string]string) http.HandlerFunc {
	currencies := money.SupportedCurrencies()
	for i := range currencies {
		if name := gatewayNames[currencies[i].Code]; name != "" {
			currencies[i].Gateway = &name
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		json.NewEncoder(w).Encode(struct {
			Currencies []money.CurrencyDefinition `json:"currencies"`
		}{currencies})
	}
}
