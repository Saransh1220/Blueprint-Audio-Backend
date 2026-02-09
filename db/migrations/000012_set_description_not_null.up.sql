UPDATE specs SET description = '' WHERE description IS NULL;
ALTER TABLE specs ALTER COLUMN description SET DEFAULT '';
ALTER TABLE specs ALTER COLUMN description SET NOT NULL;
