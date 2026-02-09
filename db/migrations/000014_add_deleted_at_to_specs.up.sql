ALTER TABLE specs ADD COLUMN deleted_at TIMESTAMP WITH TIME ZONE DEFAULT NULL;
ALTER TABLE specs ADD COLUMN is_deleted BOOLEAN DEFAULT FALSE;

CREATE INDEX idx_specs_deleted_at ON specs(deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_specs_is_deleted ON specs(is_deleted) WHERE is_deleted = FALSE;
