ALTER TABLE license_options ADD COLUMN is_deleted BOOLEAN DEFAULT FALSE;
CREATE INDEX idx_license_options_is_deleted ON license_options(is_deleted) WHERE is_deleted = FALSE;
