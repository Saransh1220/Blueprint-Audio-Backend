 ALTER TABLE specs
  ADD COLUMN IF NOT EXISTS moods TEXT[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS instruments TEXT[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS slug VARCHAR(140),
  ADD COLUMN IF NOT EXISTS short_code VARCHAR(16);

CREATE INDEX IF NOT EXISTS idx_specs_moods_gin
  ON specs USING GIN (moods);

CREATE INDEX IF NOT EXISTS idx_specs_instruments_gin
  ON specs USING GIN (instruments);

CREATE UNIQUE INDEX IF NOT EXISTS idx_specs_slug_unique
  ON specs (slug)
  WHERE slug IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_specs_short_code_unique
  ON specs (short_code)
  WHERE short_code IS NOT NULL;
