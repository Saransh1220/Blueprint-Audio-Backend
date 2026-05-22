DROP TABLE IF EXISTS payment_webhook_events;

ALTER TABLE licenses
    DROP COLUMN IF EXISTS currency;

ALTER TABLE orders
    DROP COLUMN IF EXISTS provider_payment_id,
    DROP COLUMN IF EXISTS provider_checkout_id,
    DROP COLUMN IF EXISTS provider;

DROP TABLE IF EXISTS license_option_prices;
