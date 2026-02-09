DROP INDEX IF EXISTS idx_specs_free_mp3_enabled;
ALTER TABLE specs DROP COLUMN IF EXISTS free_mp3_enabled;
ALTER TABLE specs DROP COLUMN IF EXISTS duration;
