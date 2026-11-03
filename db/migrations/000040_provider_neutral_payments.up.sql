-- Payments are no longer Razorpay-only. Store gateway references in
-- provider-neutral columns so any gateway (Razorpay, Dodo, ...) fits.

-- ORDERS ---------------------------------------------------------------
-- The gateway's id for a checkout (Razorpay order id / Dodo session id)
-- already lives in provider_checkout_id for Dodo. Move Razorpay's there too.
UPDATE orders
SET provider_checkout_id = razorpay_order_id
WHERE provider_checkout_id IS NULL
  AND razorpay_order_id IS NOT NULL;

CREATE UNIQUE INDEX idx_orders_provider_checkout_id
    ON orders (provider, provider_checkout_id)
    WHERE provider_checkout_id IS NOT NULL;

-- Also drops the unique constraint and index that were on this column.
ALTER TABLE orders DROP COLUMN razorpay_order_id;

-- PAYMENTS -------------------------------------------------------------
ALTER TABLE payments ADD COLUMN provider VARCHAR(40) NOT NULL DEFAULT 'razorpay';

-- Dodo payments were stored in the razorpay_* columns; tag them correctly.
UPDATE payments p
SET provider = o.provider
FROM orders o
WHERE o.id = p.order_id;

-- New rows must say which gateway they came from.
ALTER TABLE payments ALTER COLUMN provider DROP DEFAULT;

ALTER TABLE payments RENAME COLUMN razorpay_payment_id TO provider_payment_id;
ALTER TABLE payments RENAME COLUMN razorpay_signature TO provider_signature;

ALTER INDEX idx_payments_razorpay_payment_id RENAME TO idx_payments_provider_payment_id;
ALTER TABLE payments RENAME CONSTRAINT payments_razorpay_payment_id_key TO payments_provider_payment_id_key;

COMMENT ON COLUMN payments.provider_signature IS 'Gateway signature (Razorpay HMAC / Dodo webhook signature) kept for audit';
