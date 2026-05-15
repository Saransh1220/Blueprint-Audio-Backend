DROP INDEX IF EXISTS idx_specs_short_code_unique;
DROP INDEX IF EXISTS idx_specs_slug_unique;
DROP INDEX IF EXISTS idx_specs_instruments_gin;
DROP INDEX IF EXISTS idx_specs_moods_gin;

ALTER TABLE specs
  DROP COLUMN IF EXISTS short_code,
  DROP COLUMN IF EXISTS slug,
  DROP COLUMN IF EXISTS instruments,
  DROP COLUMN IF EXISTS moods;
