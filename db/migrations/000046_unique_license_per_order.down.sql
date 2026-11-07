DROP INDEX IF EXISTS idx_licenses_order_id;
CREATE INDEX idx_licenses_order_id ON licenses(order_id);
