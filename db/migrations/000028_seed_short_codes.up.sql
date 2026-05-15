UPDATE specs
SET short_code = substr(replace(gen_random_uuid()::text, '-', ''), 1, 8)
WHERE short_code IS NULL OR short_code = '';
