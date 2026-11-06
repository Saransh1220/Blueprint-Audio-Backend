package priceaudit

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

type RowClassification string

const (
	ClassVerifiedNative      RowClassification = "VERIFIED_NATIVE"
	ClassMissingNative       RowClassification = "MISSING_NATIVE"
	ClassStaleLegacyMismatch RowClassification = "STALE_LEGACY_MISMATCH"
	ClassOwnerManualConflict RowClassification = "OWNER_MANUAL_CONFLICT"
	ClassInactivePrice       RowClassification = "INACTIVE_PRICE"
	ClassInvalidAmount       RowClassification = "INVALID_AMOUNT"
	ClassMislabeledMig30     RowClassification = "MISLABELED_MIGRATION_30"
	ClassExplicitSecondary   RowClassification = "EXPLICIT_SECONDARY"
	ClassMissingSecondary    RowClassification = "MISSING_SECONDARY"
)

type OptionAuditRow struct {
	LicenseOptionID uuid.UUID           `json:"license_option_id"`
	SpecID          uuid.UUID           `json:"spec_id"`
	OptionName      string              `json:"option_name"`
	NativePrice     float64             `json:"native_price"`
	NativeCurrency  string              `json:"native_currency"`
	Classifications []RowClassification `json:"classifications"`
	ExistingPrices  []money.StoredPrice `json:"existing_prices"`
	ProposedActions []string            `json:"proposed_actions"`
	ExpectedBefore  *money.StoredPrice  `json:"expected_before,omitempty"`
}

type AuditSummary struct {
	TotalOptionsChecked      int              `json:"total_options_checked"`
	VerifiedNativeCount      int              `json:"verified_native_count"`
	MissingNativeCount       int              `json:"missing_native_count"`
	StaleLegacyMismatchCount int              `json:"stale_legacy_mismatch_count"`
	OwnerManualConflictCount int              `json:"owner_manual_conflict_count"`
	InactivePriceCount       int              `json:"inactive_price_count"`
	InvalidAmountCount       int              `json:"invalid_amount_count"`
	MislabeledMig30Count     int              `json:"mislabeled_mig30_count"`
	ExplicitSecondaryCount   int              `json:"explicit_secondary_count"`
	MissingSecondaryCount    int              `json:"missing_secondary_count"`
	ProposedNativeInsertions int              `json:"proposed_native_insertions"`
	ProposedNativeUpdates    int              `json:"proposed_native_updates"`
	Details                  []OptionAuditRow `json:"details,omitempty"`
}

// AuditOption evaluates a license option and its existing stored price rows.
func AuditOption(
	optionID, specID uuid.UUID,
	optionName string,
	nativePrice float64,
	nativeCurrency string,
	existingPrices []money.StoredPrice,
) OptionAuditRow {
	natCurr := strings.ToUpper(strings.TrimSpace(nativeCurrency))
	if natCurr == "" {
		natCurr = money.CurrencyINR
	}

	row := OptionAuditRow{
		LicenseOptionID: optionID,
		SpecID:          specID,
		OptionName:      optionName,
		NativePrice:     nativePrice,
		NativeCurrency:  natCurr,
		Classifications: make([]RowClassification, 0),
		ExistingPrices:  existingPrices,
		ProposedActions: make([]string, 0),
	}

	expectedNativeMinor, err := QuantizeDecimal(nativePrice)
	if err != nil {
		row.Classifications = append(row.Classifications, ClassInvalidAmount)
		row.ProposedActions = append(row.ProposedActions, fmt.Sprintf("Flag invalid native price decimal %.2f: %v", nativePrice, err))
		return row
	}

	var nativePriceRow *money.StoredPrice
	var hasMislabeledMig30 bool
	var secondaryFound bool

	for i := range existingPrices {
		p := &existingPrices[i]
		curr := strings.ToUpper(strings.TrimSpace(p.Currency))
		if curr == natCurr {
			nativePriceRow = p
		} else {
			if p.Source == "legacy_price" && natCurr == money.CurrencyUSD && curr == money.CurrencyINR {
				hasMislabeledMig30 = true
			} else if p.IsActive {
				secondaryFound = true
			}
		}
	}

	if nativePriceRow == nil {
		row.Classifications = append(row.Classifications, ClassMissingNative)
		row.ProposedActions = append(row.ProposedActions, fmt.Sprintf("Insert missing canonical native %s row at %d minor units (rev 1)", natCurr, expectedNativeMinor))
	} else if !nativePriceRow.IsActive {
		row.Classifications = append(row.Classifications, ClassInactivePrice)
		row.ProposedActions = append(row.ProposedActions, "Native price row is deactivated; preserve inactive state unless manual reactivation is explicitly approved")
	} else if nativePriceRow.AmountMinor == expectedNativeMinor {
		row.Classifications = append(row.Classifications, ClassVerifiedNative)
	} else {
		// Amount mismatch between legacy scalar and stored price row
		row.ExpectedBefore = nativePriceRow
		if nativePriceRow.Source != "legacy_price" || nativePriceRow.Revision > 1 {
			row.Classifications = append(row.Classifications, ClassOwnerManualConflict)
			row.ProposedActions = append(row.ProposedActions, fmt.Sprintf("Retain owner-edited price %d minor units (rev %d); DO NOT overwrite with legacy scalar %.2f", nativePriceRow.AmountMinor, nativePriceRow.Revision, nativePrice))
		} else {
			row.Classifications = append(row.Classifications, ClassStaleLegacyMismatch)
			row.ProposedActions = append(row.ProposedActions, fmt.Sprintf("Update legacy row (rev %d, was %d minor) to %d minor units conditionally via manifest", nativePriceRow.Revision, nativePriceRow.AmountMinor, expectedNativeMinor))
		}
	}

	if hasMislabeledMig30 {
		row.Classifications = append(row.Classifications, ClassMislabeledMig30)
		row.ProposedActions = append(row.ProposedActions, "Flag mislabeled legacy INR row for review/deactivation")
	}

	if secondaryFound {
		row.Classifications = append(row.Classifications, ClassExplicitSecondary)
	} else {
		row.Classifications = append(row.Classifications, ClassMissingSecondary)
		row.ProposedActions = append(row.ProposedActions, "Secondary currency absent; retain absent state until dated FX manifest approved")
	}

	return row
}

// CompileAuditSummary aggregates row results into a structured summary.
func CompileAuditSummary(rows []OptionAuditRow) AuditSummary {
	summary := AuditSummary{
		TotalOptionsChecked: len(rows),
		Details:             rows,
	}

	for _, r := range rows {
		for _, c := range r.Classifications {
			switch c {
			case ClassVerifiedNative:
				summary.VerifiedNativeCount++
			case ClassMissingNative:
				summary.MissingNativeCount++
				summary.ProposedNativeInsertions++
			case ClassStaleLegacyMismatch:
				summary.StaleLegacyMismatchCount++
				summary.ProposedNativeUpdates++
			case ClassOwnerManualConflict:
				summary.OwnerManualConflictCount++
			case ClassInactivePrice:
				summary.InactivePriceCount++
			case ClassInvalidAmount:
				summary.InvalidAmountCount++
			case ClassMislabeledMig30:
				summary.MislabeledMig30Count++
			case ClassExplicitSecondary:
				summary.ExplicitSecondaryCount++
			case ClassMissingSecondary:
				summary.MissingSecondaryCount++
			}
		}
	}

	return summary
}

// FormatAuditJSON formats the summary as clean JSON.
func FormatAuditJSON(summary AuditSummary) (string, error) {
	bytes, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// QuantizeDecimal quantizes a legacy float safely to cents/paise with overflow checking.
func QuantizeDecimal(amount float64) (int, error) {
	if amount < 0 {
		return 0, errors.New("negative amount not allowed")
	}
	return money.MajorToMinor(amount)
}
