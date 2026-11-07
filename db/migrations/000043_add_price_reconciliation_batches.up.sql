-- Recovery evidence for explicitly reviewed operational price changes.
CREATE TABLE price_reconciliation_batches (
    id UUID PRIMARY KEY,
    manifest JSONB NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    rolled_back_at TIMESTAMPTZ
);
CREATE TABLE price_reconciliation_changes (
    batch_id UUID NOT NULL REFERENCES price_reconciliation_batches(id),
    price_id UUID NOT NULL,
    before_row JSONB,
    after_row JSONB NOT NULL,
    PRIMARY KEY (batch_id, price_id)
);
