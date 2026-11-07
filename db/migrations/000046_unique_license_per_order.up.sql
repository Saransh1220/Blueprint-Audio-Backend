-- Ensure that each order can have at most one active license issued.
-- First, verify no duplicates currently exist; if any do, abort the migration with a clear error.
DO $$
BEGIN
    IF EXISTS (
        SELECT order_id
        FROM licenses
        GROUP BY order_id
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'Cannot apply migration 000046: duplicate licenses exist for at least one order. Please inspect and resolve duplicates manually before migrating.';
    END IF;
END $$;

DROP INDEX IF EXISTS idx_licenses_order_id;
CREATE UNIQUE INDEX idx_licenses_order_id ON licenses(order_id);
