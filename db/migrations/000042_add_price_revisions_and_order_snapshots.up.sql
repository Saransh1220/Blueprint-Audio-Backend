-- Migration 000042: Add price revisions, active flags, and indexes to support explicit pricing

ALTER TABLE license_option_prices
    ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;

-- Ensure consistency between amount_minor and amount_major
UPDATE license_option_prices
SET amount_major = ROUND(amount_minor::numeric / 100.0, 2)
WHERE amount_minor >= 0 AND (amount_major IS NULL OR amount_major != ROUND(amount_minor::numeric / 100.0, 2));

CREATE INDEX IF NOT EXISTS idx_license_option_prices_lookup
    ON license_option_prices (license_option_id, currency, is_active);

CREATE INDEX IF NOT EXISTS idx_license_option_prices_active
    ON license_option_prices (license_option_id)
    WHERE is_active = true;
