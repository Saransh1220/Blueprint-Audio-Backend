ALTER TABLE users ADD COLUMN store_currency VARCHAR(3) NOT NULL DEFAULT 'USD' CHECK (store_currency IN ('INR', 'USD'));
