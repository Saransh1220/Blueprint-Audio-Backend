CREATE TABLE license_option_prices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    license_option_id UUID NOT NULL REFERENCES license_options(id) ON DELETE CASCADE,
    currency VARCHAR(3) NOT NULL CHECK (currency IN ('INR', 'USD')),
    amount_minor INTEGER NOT NULL CHECK (amount_minor >= 0),
    amount_major DECIMAL(12,2) NOT NULL CHECK (amount_major >= 0),
    source VARCHAR(40) NOT NULL DEFAULT 'backfill',
    fx_rate_snapshot DECIMAL(18,8),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE (license_option_id, currency)
);

INSERT INTO license_option_prices (
    license_option_id,
    currency,
    amount_minor,
    amount_major,
    source
)
SELECT
    id,
    'INR',
    ROUND(price * 100)::INTEGER,
    price,
    'legacy_price'
FROM license_options
ON CONFLICT (license_option_id, currency) DO NOTHING;

ALTER TABLE orders
    ADD COLUMN provider VARCHAR(40) NOT NULL DEFAULT 'razorpay',
    ADD COLUMN provider_checkout_id VARCHAR(255),
    ADD COLUMN provider_payment_id VARCHAR(255);

ALTER TABLE licenses
    ADD COLUMN currency VARCHAR(3) NOT NULL DEFAULT 'INR';

CREATE TABLE payment_webhook_events (
    id VARCHAR(255) PRIMARY KEY,
    provider VARCHAR(40) NOT NULL,
    event_type VARCHAR(120) NOT NULL,
    processed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
