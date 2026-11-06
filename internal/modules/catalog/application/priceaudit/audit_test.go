package priceaudit

import (
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditOption(t *testing.T) {
	optID := uuid.New()
	specID := uuid.New()

	t.Run("Option with verified native INR and missing secondary", func(t *testing.T) {
		existing := []money.StoredPrice{
			{
				ID:              uuid.New(),
				LicenseOptionID: optID,
				Currency:        "INR",
				AmountMinor:     100000,
				AmountMajor:     1000.00,
				Source:          "owner",
				Revision:        1,
				IsActive:        true,
			},
		}

		res := AuditOption(optID, specID, "Basic", 1000.00, "INR", existing)
		assert.Contains(t, res.Classifications, ClassVerifiedNative)
		assert.Contains(t, res.Classifications, ClassMissingSecondary)
		assert.NotContains(t, res.Classifications, ClassMissingNative)
	})

	t.Run("Option with stale native price and explicit secondary USD", func(t *testing.T) {
		existing := []money.StoredPrice{
			{
				ID:              uuid.New(),
				LicenseOptionID: optID,
				Currency:        "INR",
				AmountMinor:     50000, // Stale legacy! Option says 1000.00 (100000)
				AmountMajor:     500.00,
				Source:          "legacy_price",
				Revision:        1,
				IsActive:        true,
			},
			{
				ID:              uuid.New(),
				LicenseOptionID: optID,
				Currency:        "USD",
				AmountMinor:     1499,
				AmountMajor:     14.99,
				Source:          "owner",
				Revision:        2,
				IsActive:        true,
			},
		}

		res := AuditOption(optID, specID, "Premium", 1000.00, "INR", existing)
		assert.Contains(t, res.Classifications, ClassStaleLegacyMismatch)
		assert.Contains(t, res.Classifications, ClassExplicitSecondary)
		require.NotNil(t, res.ExpectedBefore)
		assert.Equal(t, 50000, res.ExpectedBefore.AmountMinor)
	})

	t.Run("Option with owner manual edit must be protected from overwrite", func(t *testing.T) {
		existing := []money.StoredPrice{
			{
				ID:              uuid.New(),
				LicenseOptionID: optID,
				Currency:        "INR",
				AmountMinor:     150000, // Owner edited price to 1500
				AmountMajor:     1500.00,
				Source:          "owner",
				Revision:        2,
				IsActive:        true,
			},
		}

		res := AuditOption(optID, specID, "Unlimited", 1000.00, "INR", existing)
		assert.Contains(t, res.Classifications, ClassOwnerManualConflict)
		assert.NotContains(t, res.Classifications, ClassStaleLegacyMismatch)
		assert.Contains(t, res.ProposedActions[0], "Retain owner-edited price")
	})

	t.Run("Option with missing native row", func(t *testing.T) {
		res := AuditOption(optID, specID, "Trackout", 2000.00, "INR", nil)
		assert.Contains(t, res.Classifications, ClassMissingNative)
		assert.Contains(t, res.Classifications, ClassMissingSecondary)
	})

	t.Run("Option with mislabeled migration 30 INR row on USD native option", func(t *testing.T) {
		existing := []money.StoredPrice{
			{
				ID:              uuid.New(),
				LicenseOptionID: optID,
				Currency:        "INR",
				AmountMinor:     2500, // Stored as 2500 paise, but native is $25.00
				AmountMajor:     25.00,
				Source:          "legacy_price",
				Revision:        1,
				IsActive:        true,
			},
		}

		res := AuditOption(optID, specID, "Unlimited", 25.00, "USD", existing)
		assert.Contains(t, res.Classifications, ClassMislabeledMig30)
		assert.Contains(t, res.Classifications, ClassMissingNative) // Real USD native row is missing
	})
}

func TestQuantizeDecimal(t *testing.T) {
	minor, err := QuantizeDecimal(10.50)
	require.NoError(t, err)
	assert.Equal(t, 1050, minor)

	_, err = QuantizeDecimal(-5.0)
	assert.Error(t, err)

	_, err = QuantizeDecimal(1e12)
	assert.Error(t, err)
}

func TestCompileAuditSummary(t *testing.T) {
	rows := []OptionAuditRow{
		{
			Classifications: []RowClassification{ClassVerifiedNative, ClassMissingSecondary},
		},
		{
			Classifications: []RowClassification{ClassMissingNative, ClassMissingSecondary},
		},
		{
			Classifications: []RowClassification{ClassOwnerManualConflict},
		},
	}

	summary := CompileAuditSummary(rows)
	assert.Equal(t, 3, summary.TotalOptionsChecked)
	assert.Equal(t, 1, summary.VerifiedNativeCount)
	assert.Equal(t, 1, summary.MissingNativeCount)
	assert.Equal(t, 1, summary.OwnerManualConflictCount)
	assert.Equal(t, 1, summary.ProposedNativeInsertions)

	jsonStr, err := FormatAuditJSON(summary)
	require.NoError(t, err)
	assert.Contains(t, jsonStr, `"verified_native_count": 1`)
	assert.Contains(t, jsonStr, `"owner_manual_conflict_count": 1`)
}

func TestAuditProtectsUnknownProvenanceEvenAtFirstRevision(t *testing.T) {
	id := uuid.New()
	row := AuditOption(id, uuid.New(), "Basic", 10, "USD", []money.StoredPrice{{ID: uuid.New(), LicenseOptionID: id, Currency: "USD", AmountMinor: 2500, Revision: 1, IsActive: true, Source: "manual_import"}})
	assert.Contains(t, row.Classifications, ClassOwnerManualConflict)
	assert.NotContains(t, row.Classifications, ClassStaleLegacyMismatch)
}
