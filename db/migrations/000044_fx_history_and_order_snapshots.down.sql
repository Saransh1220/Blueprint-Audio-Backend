UPDATE roles
SET permissions = array_remove(permissions, 'admin:fx:write'),
    updated_at = NOW()
WHERE name = 'super_admin';

UPDATE roles
SET permissions = array_remove(permissions, 'admin:fx:read'),
    updated_at = NOW()
WHERE name = 'super_admin';

ALTER TABLE orders DROP CONSTRAINT IF EXISTS check_orders_report_fx_all_or_nothing;

ALTER TABLE orders
    DROP COLUMN IF EXISTS report_fx_rate_id,
    DROP COLUMN IF EXISTS report_fx_fetched_at,
    DROP COLUMN IF EXISTS report_fx_as_of,
    DROP COLUMN IF EXISTS report_fx_rate,
    DROP COLUMN IF EXISTS report_currency;

DROP TABLE IF EXISTS fx_rates;
