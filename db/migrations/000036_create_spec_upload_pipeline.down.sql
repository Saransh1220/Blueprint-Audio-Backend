DROP TABLE IF EXISTS spec_processing_jobs;
DROP TABLE IF EXISTS spec_upload_assets;
DROP TABLE IF EXISTS spec_upload_sessions;

ALTER TABLE specs
    DROP CONSTRAINT IF EXISTS specs_bpm_check;

ALTER TABLE specs
    ADD CONSTRAINT specs_bpm_check CHECK (bpm >= 60 AND bpm <= 200);
