-- Rollback: Remove price_currency columns
ALTER TABLE license_options DROP COLUMN IF EXISTS price_currency;
ALTER TABLE specs DROP COLUMN IF EXISTS price_currency;
