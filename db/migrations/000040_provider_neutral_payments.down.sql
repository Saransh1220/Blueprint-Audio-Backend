-- Restore the Razorpay-specific columns.

-- PAYMENTS
COMMENT ON COLUMN payments.provider_signature IS NULL;

ALTER TABLE payments RENAME CONSTRAINT payments_provider_payment_id_key TO payments_razorpay_payment_id_key;
ALTER INDEX idx_payments_provider_payment_id RENAME TO idx_payments_razorpay_payment_id;

ALTER TABLE payments RENAME COLUMN provider_signature TO razorpay_signature;
ALTER TABLE payments RENAME COLUMN provider_payment_id TO razorpay_payment_id;

ALTER TABLE payments DROP COLUMN provider;

-- ORDERS
ALTER TABLE orders ADD COLUMN razorpay_order_id VARCHAR(255) UNIQUE;

UPDATE orders
SET razorpay_order_id = provider_checkout_id,
    provider_checkout_id = NULL
WHERE provider = 'razorpay';

DROP INDEX IF EXISTS idx_orders_provider_checkout_id;
