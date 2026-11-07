ALTER TABLE users ADD COLUMN preferred_currency VARCHAR(3) NULL CHECK (preferred_currency IS NULL OR preferred_currency IN ('INR', 'USD'));
