package money

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestINRFromUSDMajorAppliesMarketplaceRounding(t *testing.T) {
	oldGetenv := getenv
	defer func() { getenv = oldGetenv }()
	getenv = func(string) string { return "0.012" }

	got := INRFromUSDMajor(0.99)

	assert.Equal(t, 8300, got.AmountMinor)
	assert.Equal(t, 83.00, got.AmountMajor)
	assert.Equal(t, CurrencyINR, got.Currency)
}

func TestUSDPerINRLogsInvalidConfiguredRateAndFallsBack(t *testing.T) {
	oldGetenv := getenv
	defer func() { getenv = oldGetenv }()
	getenv = func(string) string { return "bad-rate" }

	var buf bytes.Buffer
	oldWriter := log.Writer()
	defer log.SetOutput(oldWriter)
	log.SetOutput(&buf)

	got := usdPerINR()

	assert.Equal(t, 0.012, got)
	logged := buf.String()
	assert.Contains(t, logged, "usdPerINR()")
	assert.Contains(t, logged, `INR_USD_RATE="bad-rate"`)
	assert.Contains(t, logged, "using fallback 0.012")
	assert.True(t, strings.Contains(logged, "parse_error=") || strings.Contains(logged, "parsed_rate="))
}
