-- Migration 000042 Down

DROP INDEX IF EXISTS idx_license_option_prices_active;
DROP INDEX IF EXISTS idx_license_option_prices_lookup;

ALTER TABLE license_option_prices
    DROP COLUMN IF EXISTS is_active,
    DROP COLUMN IF EXISTS revision;
