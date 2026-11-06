-- Refuse to erase recovery evidence for applied batches.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM price_reconciliation_batches) THEN
        RAISE EXCEPTION 'Retain reconciliation history; use forward correction instead of dropping audit tables';
    END IF;
END $$;
DROP TABLE price_reconciliation_changes;
DROP TABLE price_reconciliation_batches;
