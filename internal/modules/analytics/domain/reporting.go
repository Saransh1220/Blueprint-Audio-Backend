package domain

import (
	"context"

	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

type reportCurrencyKey struct{}

func WithReportCurrency(ctx context.Context, currency money.CurrencyDefinition) context.Context {
	return context.WithValue(ctx, reportCurrencyKey{}, currency)
}

func ReportCurrency(ctx context.Context) money.CurrencyDefinition {
	if currency, ok := ctx.Value(reportCurrencyKey{}).(money.CurrencyDefinition); ok {
		return currency
	}
	currency, _ := money.LookupCurrency(money.CurrencyINR)
	return currency
}
