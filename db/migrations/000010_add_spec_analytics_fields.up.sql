-- Add duration and free MP3 toggle to specs table
ALTER TABLE specs ADD COLUMN duration INTEGER DEFAULT 0; -- in seconds
ALTER TABLE specs ADD COLUMN free_mp3_enabled BOOLEAN DEFAULT false;

-- Create index for filtering free beats
CREATE INDEX idx_specs_free_mp3_enabled ON specs(free_mp3_enabled) WHERE free_mp3_enabled = true;

-- Create analytics records for existing specs
INSERT INTO spec_analytics (spec_id)
SELECT id FROM specs
ON CONFLICT (spec_id) DO NOTHING;
