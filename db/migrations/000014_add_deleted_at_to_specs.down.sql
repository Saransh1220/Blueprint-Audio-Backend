DROP INDEX IF EXISTS idx_specs_is_deleted;
DROP INDEX IF EXISTS idx_specs_deleted_at;
ALTER TABLE specs DROP COLUMN is_deleted;
ALTER TABLE specs DROP COLUMN deleted_at;
