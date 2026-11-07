package application

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

// prepareLicensePrices preserves native currency and untouched currency entries.
// Explicit patches use the same validation path as the existing scalar editor.
func prepareLicensePrices(spec *domain.Spec, existing *domain.Spec) error {
	if spec.PriceCurrency == "" && existing != nil {
		spec.PriceCurrency = existing.PriceCurrency
	}
	for i := range spec.Licenses {
		lic := &spec.Licenses[i]
		var old *domain.LicenseOption
		if existing != nil {
			for j := range existing.Licenses {
				candidate := &existing.Licenses[j]
				if (lic.ID != uuid.Nil && lic.ID == candidate.ID) || (lic.ID == uuid.Nil && lic.LicenseType == candidate.LicenseType) {
					old = candidate
					break
				}
			}
		}
		if lic.PriceCurrency == "" && old != nil {
			lic.PriceCurrency = old.PriceCurrency
		}
	}
	if err := normalizeSpecCurrencies(spec); err != nil {
		return err
	}
	for i := range spec.Licenses {
		lic := &spec.Licenses[i]
		def, ok := money.LookupCurrency(lic.PriceCurrency)
		if !ok {
			return fmt.Errorf("license %s: invalid currency %s", lic.Name, lic.PriceCurrency)
		}
		minor, err := money.MajorToMinorScale(lic.Price, def.MinorUnitScale)
		if err != nil {
			return fmt.Errorf("license %s: %w", lic.Name, err)
		}
		var prior []money.StoredPrice
		if existing != nil {
			for _, old := range existing.Licenses {
				if (lic.ID != uuid.Nil && lic.ID == old.ID) || (lic.ID == uuid.Nil && lic.LicenseType == old.LicenseType) {
					prior = old.Prices
					break
				}
			}
		}
		patches := append([]money.StoredPrice(nil), lic.Prices...)
		merged := append([]money.StoredPrice(nil), prior...)
		// The scalar authoring contract only updates the native amount. Inactive
		// rows stay inactive unless the owner explicitly changes availability.
		nativeFound := false
		for j := range merged {
			if strings.EqualFold(merged[j].Currency, lic.PriceCurrency) {
				if merged[j].AmountMinor != minor {
					merged[j].Source = "owner"
				}
				merged[j].AmountMinor = minor
				nativeFound = true
			}
		}
		if !nativeFound {
			merged = append(merged, money.StoredPrice{Currency: lic.PriceCurrency, AmountMinor: minor, Revision: 1, IsActive: true, Source: "owner"})
		}
		seen := make(map[string]bool)
		for _, patch := range patches {
			expectedRevision := patch.Revision
			if err := patch.Validate(); err != nil {
				return err
			}
			if existing == nil && strings.EqualFold(patch.Currency, lic.PriceCurrency) && minor > 0 && patch.AmountMinor != minor {
				return fmt.Errorf("conflicting price for %s: scalar price does not match prices entry", lic.PriceCurrency)
			}
			if seen[patch.Currency] {
				return fmt.Errorf("duplicate price currency %s", patch.Currency)
			}
			seen[patch.Currency] = true
			matched := false
			for j := range merged {
				if merged[j].Currency == patch.Currency {
					if expectedRevision > 0 && expectedRevision != merged[j].Revision {
						return money.ErrStalePrice
					}
					merged[j].AmountMinor = patch.AmountMinor
					merged[j].IsActive = patch.IsActive
					merged[j].Source = "owner"
					matched = true
					break
				}
			}
			if !matched {
				patch.ID = uuid.Nil
				patch.Revision = 1
				patch.Source = "owner"
				merged = append(merged, patch)
			}
		}
		for j := range merged {
			if err := merged[j].Validate(); err != nil {
				return err
			}
			if merged[j].Currency == lic.PriceCurrency {
				lic.Price = merged[j].AmountMajor
			}
		}
		lic.Prices = merged
	}
	return nil
}
