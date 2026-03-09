ALTER TABLE users ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE genres ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE specs ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE license_options ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE orders ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE payments ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE licenses ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE analytics_events ALTER COLUMN id SET DEFAULT gen_random_uuid();
