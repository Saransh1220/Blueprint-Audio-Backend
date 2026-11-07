package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

type buyerCurrencyKey string

const contextKeyBuyerResolution buyerCurrencyKey = "buyer_currency_resolution"

// PreferenceLookup loads the preferred currency for an authenticated user.
type PreferenceLookup func(ctx context.Context, userID uuid.UUID) (*string, error)

type BuyerCurrencyMiddleware struct {
	preferenceLookup PreferenceLookup
	geoTrusted       bool
}

// NewBuyerCurrencyMiddleware constructs the currency resolution middleware.
func NewBuyerCurrencyMiddleware(lookup PreferenceLookup, geoTrusted bool) *BuyerCurrencyMiddleware {
	return &BuyerCurrencyMiddleware{
		preferenceLookup: lookup,
		geoTrusted:       geoTrusted,
	}
}

// WithBuyerCurrency attaches a resolution result to the context.
func WithBuyerCurrency(ctx context.Context, res money.ResolutionResult) context.Context {
	return context.WithValue(ctx, contextKeyBuyerResolution, res)
}

// BuyerCurrencyFromContext returns the resolved 3-letter currency code, defaulting to USD.
func BuyerCurrencyFromContext(ctx context.Context) string {
	if ctx == nil {
		return money.CurrencyUSD
	}
	if res, ok := ctx.Value(contextKeyBuyerResolution).(money.ResolutionResult); ok && res.Currency != "" {
		return res.Currency
	}
	return money.CurrencyUSD
}

// BuyerResolutionFromContext returns the full resolution details including source.
func BuyerResolutionFromContext(ctx context.Context) money.ResolutionResult {
	if ctx == nil {
		return money.ResolutionResult{Currency: money.CurrencyUSD, Source: money.SourceDefault}
	}
	if res, ok := ctx.Value(contextKeyBuyerResolution).(money.ResolutionResult); ok && res.Currency != "" {
		return res
	}
	return money.ResolutionResult{Currency: money.CurrencyUSD, Source: money.SourceDefault}
}

// Middleware resolves buyer currency according to D2 precedence and sets Vary headers.
func (m *BuyerCurrencyMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Cookie")
		w.Header().Add("Vary", "CF-IPCountry")
		w.Header().Add("Vary", "X-Country-Code")

		var profileCurrency string
		if m.preferenceLookup != nil {
			if userID, ok := r.Context().Value(ContextKeyUserId).(uuid.UUID); ok {
				if pref, err := m.preferenceLookup(r.Context(), userID); err == nil && pref != nil {
					profileCurrency = *pref
				}
			}
		}

		var cookieCurrency string
		if c, err := r.Cookie("wy_currency"); err == nil {
			cookieCurrency = c.Value
		}

		country := strings.TrimSpace(r.Header.Get("CF-IPCountry"))
		if country == "" {
			country = strings.TrimSpace(r.Header.Get("X-Country-Code"))
		}

		res := money.ResolveDisplayCurrency(money.ResolveInput{
			ProfileCurrency: profileCurrency,
			CookieCurrency:  cookieCurrency,
			CountryCode:     country,
		})

		ctx := WithBuyerCurrency(r.Context(), res)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
