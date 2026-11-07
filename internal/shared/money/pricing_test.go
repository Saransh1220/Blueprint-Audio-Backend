package money_test

import (
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

func TestPricing_SelectPriceOffer(t *testing.T) {
	inrPriceID := uuid.New()
	usdPriceID := uuid.New()
	zeroPriceID := uuid.New()

	inrPrice := money.StoredPrice{
		ID:          inrPriceID,
		Currency:    money.CurrencyINR,
		AmountMinor: 100000,
		AmountMajor: 1000.00,
		Revision:    1,
		IsActive:    true,
	}

	usdPrice := money.StoredPrice{
		ID:          usdPriceID,
		Currency:    money.CurrencyUSD,
		AmountMinor: 1499,
		AmountMajor: 14.99,
		Revision:    2,
		IsActive:    true,
	}

	usd1500Price := money.StoredPrice{
		ID:          usdPriceID,
		Currency:    money.CurrencyUSD,
		AmountMinor: 1500,
		AmountMajor: 15.00,
		Revision:    1,
		IsActive:    true,
	}

	zeroPrice := money.StoredPrice{
		ID:          zeroPriceID,
		Currency:    money.CurrencyUSD,
		AmountMinor: 0,
		AmountMajor: 0.00,
		Revision:    1,
		IsActive:    true,
	}

	t.Run("Stored INR and USD: exact selection without FX", func(t *testing.T) {
		prices := []money.StoredPrice{inrPrice, usdPrice}

		// Request INR
		offerINR := money.SelectPriceOffer(prices, money.CurrencyINR, "INR")
		if offerINR.Selection != money.SelectionExact {
			t.Fatalf("expected exact selection, got %s", offerINR.Selection)
		}
		if offerINR.AmountMinor != 100000 || offerINR.AmountMajor != 1000.00 || offerINR.Currency != "INR" {
			t.Fatalf("unexpected INR offer: %+v", offerINR)
		}
		if *offerINR.PriceID != inrPriceID || offerINR.Revision != 1 {
			t.Fatalf("unexpected price ID or revision: %+v", offerINR)
		}

		// Request USD
		offerUSD := money.SelectPriceOffer(prices, money.CurrencyINR, "USD")
		if offerUSD.Selection != money.SelectionExact {
			t.Fatalf("expected exact selection, got %s", offerUSD.Selection)
		}
		if offerUSD.AmountMinor != 1499 || offerUSD.AmountMajor != 14.99 || offerUSD.Currency != "USD" {
			t.Fatalf("unexpected USD offer: %+v", offerUSD)
		}
	})

	t.Run("Stored USD 1500 remains exact $15.00, never rounded or altered", func(t *testing.T) {
		prices := []money.StoredPrice{usd1500Price}
		offer := money.SelectPriceOffer(prices, money.CurrencyUSD, "USD")
		if offer.AmountMinor != 1500 || offer.AmountMajor != 15.00 {
			t.Fatalf("expected 1500 / 15.00, got minor=%d major=%f", offer.AmountMinor, offer.AmountMajor)
		}
	})

	t.Run("Missing USD with native INR provides honest native fallback", func(t *testing.T) {
		prices := []money.StoredPrice{inrPrice}
		offer := money.SelectPriceOffer(prices, money.CurrencyINR, "USD")

		if offer.Selection != money.SelectionNativeFallback {
			t.Fatalf("expected native fallback, got %s", offer.Selection)
		}
		if offer.Currency != "INR" {
			t.Fatalf("expected fallback offer currency to be INR, got %s", offer.Currency)
		}
		if offer.AmountMinor != 100000 {
			t.Fatalf("expected native amount 100000, got %d", offer.AmountMinor)
		}
		if offer.FallbackReason == "" {
			t.Fatal("expected explanatory fallback reason")
		}
		if !offer.IsPurchasable {
			t.Fatal("fallback offer should be purchasable upon user confirmation")
		}
	})

	t.Run("Stored zero price is exact and valid", func(t *testing.T) {
		prices := []money.StoredPrice{zeroPrice}
		offer := money.SelectPriceOffer(prices, money.CurrencyUSD, "USD")
		if offer.Selection != money.SelectionExact {
			t.Fatalf("expected exact for zero price, got %s", offer.Selection)
		}
		if offer.AmountMinor != 0 || offer.AmountMajor != 0.00 {
			t.Fatalf("expected 0 amount, got minor=%d major=%f", offer.AmountMinor, offer.AmountMajor)
		}
		if !offer.IsPurchasable {
			t.Fatal("expected stored zero to be purchasable")
		}
	})

	t.Run("Absent row is unavailable, never free or fabricated zero", func(t *testing.T) {
		prices := []money.StoredPrice{}
		offer := money.SelectPriceOffer(prices, money.CurrencyINR, "USD")
		if offer.Selection != money.SelectionUnavailable {
			t.Fatalf("expected unavailable for absent row, got %s", offer.Selection)
		}
		if offer.IsPurchasable {
			t.Fatal("absent price must not be purchasable")
		}
	})

	t.Run("Unsupported currency rejected and never normalized to INR", func(t *testing.T) {
		prices := []money.StoredPrice{inrPrice, usdPrice}
		for _, unsupported := range []string{"EUR", "GBP", "XYZ", "usd1", "   "} {
			offer := money.SelectPriceOffer(prices, money.CurrencyINR, unsupported)
			if offer.Selection != money.SelectionUnavailable {
				t.Fatalf("expected unavailable for unsupported currency %q, got %s", unsupported, offer.Selection)
			}
			if offer.IsPurchasable {
				t.Fatalf("unsupported currency %q must not be purchasable", unsupported)
			}
		}
	})

	t.Run("Inactive prices are ignored", func(t *testing.T) {
		inactiveUSD := usdPrice
		inactiveUSD.IsActive = false
		prices := []money.StoredPrice{inrPrice, inactiveUSD}

		offer := money.SelectPriceOffer(prices, money.CurrencyINR, "USD")
		// Should fallback to native INR because USD is inactive
		if offer.Selection != money.SelectionNativeFallback {
			t.Fatalf("expected fallback to native INR when USD is inactive, got %s", offer.Selection)
		}
	})
}

func TestPricing_StoredPriceValidation(t *testing.T) {
	t.Run("Valid prices calculate major amount exactly", func(t *testing.T) {
		p := money.StoredPrice{
			Currency:    "inr",
			AmountMinor: 49900,
			Revision:    0,
		}
		if err := p.Validate(); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		if p.Currency != "INR" {
			t.Fatalf("expected uppercase INR, got %s", p.Currency)
		}
		if p.AmountMajor != 499.00 {
			t.Fatalf("expected major amount 499.00, got %f", p.AmountMajor)
		}
		if p.Revision != 1 {
			t.Fatalf("expected revision initialized to 1, got %d", p.Revision)
		}
	})

	t.Run("Negative amount rejected", func(t *testing.T) {
		p := money.StoredPrice{
			Currency:    "INR",
			AmountMinor: -50,
		}
		if err := p.Validate(); err != money.ErrNegativeAmount {
			t.Fatalf("expected ErrNegativeAmount, got %v", err)
		}
	})

	t.Run("Overflow amount rejected", func(t *testing.T) {
		p := money.StoredPrice{
			Currency:    "USD",
			AmountMinor: money.MaxAmountMinor + 1,
		}
		if err := p.Validate(); err != money.ErrAmountOverflow {
			t.Fatalf("expected ErrAmountOverflow, got %v", err)
		}
	})

	t.Run("Invalid currency rejected", func(t *testing.T) {
		p := money.StoredPrice{
			Currency:    "EUR",
			AmountMinor: 1000,
		}
		if err := p.Validate(); err != money.ErrInvalidCurrency {
			t.Fatalf("expected ErrInvalidCurrency, got %v", err)
		}
	})
}

func TestPricing_MajorToMinor(t *testing.T) {
	t.Run("Standard conversions", func(t *testing.T) {
		minor, err := money.MajorToMinor(14.99)
		if err != nil || minor != 1499 {
			t.Fatalf("expected 1499, got %d (err: %v)", minor, err)
		}

		minor, err = money.MajorToMinor(15.00)
		if err != nil || minor != 1500 {
			t.Fatalf("expected 1500, got %d (err: %v)", minor, err)
		}

		minor, err = money.MajorToMinor(0.0)
		if err != nil || minor != 0 {
			t.Fatalf("expected 0, got %d (err: %v)", minor, err)
		}
	})

	t.Run("Negative amount rejected", func(t *testing.T) {
		_, err := money.MajorToMinor(-1.0)
		if err != money.ErrNegativeAmount {
			t.Fatalf("expected ErrNegativeAmount, got %v", err)
		}
	})
}

func TestPricing_ScaleAwareConversions(t *testing.T) {
	t.Run("MinorToMajor with various scales", func(t *testing.T) {
		// Standard 2-decimal (scale 100)
		if got := money.MinorToMajor(1499, 100); got != 14.99 {
			t.Fatalf("expected 14.99, got %f", got)
		}
		// 0-decimal (scale 1, like JPY)
		if got := money.MinorToMajor(1500, 1); got != 1500.0 {
			t.Fatalf("expected 1500.0, got %f", got)
		}
		// 3-decimal (scale 1000, like BHD)
		if got := money.MinorToMajor(12345, 1000); got != 12.345 {
			t.Fatalf("expected 12.345, got %f", got)
		}
		// Zero or negative scale falls back to 100
		if got := money.MinorToMajor(5000, 0); got != 50.0 {
			t.Fatalf("expected 50.0 fallback for scale 0, got %f", got)
		}
		if got := money.MinorToMajor(5000, -10); got != 50.0 {
			t.Fatalf("expected 50.0 fallback for negative scale, got %f", got)
		}
	})

	t.Run("MajorToMinorScale with various scales", func(t *testing.T) {
		// Scale 1 (0-decimal)
		minor, err := money.MajorToMinorScale(2500.0, 1)
		if err != nil || minor != 2500 {
			t.Fatalf("expected 2500, got %d (err: %v)", minor, err)
		}

		// Scale 100 (2-decimal)
		minor, err = money.MajorToMinorScale(29.99, 100)
		if err != nil || minor != 2999 {
			t.Fatalf("expected 2999, got %d (err: %v)", minor, err)
		}

		// Scale 1000 (3-decimal)
		minor, err = money.MajorToMinorScale(1.234, 1000)
		if err != nil || minor != 1234 {
			t.Fatalf("expected 1234, got %d (err: %v)", minor, err)
		}

		// Fallback for invalid scale
		minor, err = money.MajorToMinorScale(10.0, 0)
		if err != nil || minor != 1000 {
			t.Fatalf("expected 1000 (fallback 100 scale), got %d (err: %v)", minor, err)
		}
	})

	t.Run("Error handling in MajorToMinorScale", func(t *testing.T) {
		// NaN
		_, err := money.MajorToMinorScale(math.NaN(), 100)
		if err == nil {
			t.Fatal("expected error for NaN")
		}

		// Inf
		_, err = money.MajorToMinorScale(math.Inf(1), 100)
		if err == nil {
			t.Fatal("expected error for Inf")
		}

		// Negative
		_, err = money.MajorToMinorScale(-5.0, 100)
		if err != money.ErrNegativeAmount {
			t.Fatalf("expected ErrNegativeAmount, got %v", err)
		}

		// Overflow
		_, err = money.MajorToMinorScale(float64(money.MaxAmountMinor)+1000, 100)
		if err != money.ErrAmountOverflow {
			t.Fatalf("expected ErrAmountOverflow, got %v", err)
		}
	})
}
