DROP INDEX IF EXISTS idx_license_options_is_deleted;
ALTER TABLE license_options DROP COLUMN is_deleted;
