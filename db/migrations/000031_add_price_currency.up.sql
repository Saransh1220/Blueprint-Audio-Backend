-- Add price_currency column to specs table (default INR for existing records)
ALTER TABLE specs ADD COLUMN IF NOT EXISTS price_currency VARCHAR(3) NOT NULL DEFAULT 'INR';

-- Add price_currency column to license_options table (default INR for existing records)
ALTER TABLE license_options ADD COLUMN IF NOT EXISTS price_currency VARCHAR(3) NOT NULL DEFAULT 'INR';
