CREATE TABLE IF NOT EXISTS fx_rates (
    id BIGSERIAL PRIMARY KEY,
    base VARCHAR(3) NOT NULL,
    quote VARCHAR(3) NOT NULL,
    rate NUMERIC(18,8) NOT NULL CHECK (rate > 0),
    source TEXT NOT NULL,
    as_of TIMESTAMPTZ NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    status VARCHAR(20) NOT NULL DEFAULT 'accepted' CHECK (status IN ('accepted', 'quarantined')),
    CONSTRAINT check_fx_rates_distinct CHECK (base <> quote),
    CONSTRAINT check_fx_rates_base_format CHECK (base ~ '^[A-Z]{3}$'),
    CONSTRAINT check_fx_rates_quote_format CHECK (quote ~ '^[A-Z]{3}$')
);

CREATE INDEX IF NOT EXISTS idx_fx_rates_pair_fetched ON fx_rates(base, quote, fetched_at DESC);
CREATE INDEX IF NOT EXISTS idx_fx_rates_status ON fx_rates(status);

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS report_currency VARCHAR(3),
    ADD COLUMN IF NOT EXISTS report_fx_rate NUMERIC(18,8),
    ADD COLUMN IF NOT EXISTS report_fx_as_of TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS report_fx_fetched_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS report_fx_rate_id BIGINT REFERENCES fx_rates(id);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'check_orders_report_fx_all_or_nothing'
    ) THEN
        ALTER TABLE orders ADD CONSTRAINT check_orders_report_fx_all_or_nothing
            CHECK ((report_fx_rate IS NULL) = (report_currency IS NULL));
    END IF;
END $$;

UPDATE roles
SET permissions = array_append(permissions, 'admin:fx:read'),
    updated_at = NOW()
WHERE name = 'super_admin' AND NOT ('admin:fx:read' = ANY(permissions));

UPDATE roles
SET permissions = array_append(permissions, 'admin:fx:write'),
    updated_at = NOW()
WHERE name = 'super_admin' AND NOT ('admin:fx:write' = ANY(permissions));
