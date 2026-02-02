ALTER TABLE specs ADD COLUMN IF NOT EXISTS tags TEXT[] DEFAULT '{}';
-- GIN Indexes are super fast for searching arrays!
CREATE INDEX idx_specs_tags ON specs USING GIN(tags);