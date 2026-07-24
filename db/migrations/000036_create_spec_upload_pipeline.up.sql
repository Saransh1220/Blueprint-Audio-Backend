ALTER TABLE specs
    DROP CONSTRAINT IF EXISTS specs_bpm_check;

ALTER TABLE specs
    ADD CONSTRAINT specs_bpm_check CHECK (bpm >= 60 AND bpm <= 300);

CREATE TABLE spec_upload_sessions (
    id UUID PRIMARY KEY,
    spec_id UUID NOT NULL UNIQUE,
    producer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    metadata JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'uploading'
        CHECK (status IN ('uploading', 'queued', 'processing', 'completed', 'failed', 'expired')),
    error_message TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_spec_upload_sessions_producer
    ON spec_upload_sessions (producer_id, created_at DESC);

CREATE INDEX idx_spec_upload_sessions_expiry
    ON spec_upload_sessions (expires_at)
    WHERE status = 'uploading';

CREATE TABLE spec_upload_assets (
    id UUID PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES spec_upload_sessions(id) ON DELETE CASCADE,
    kind VARCHAR(20) NOT NULL CHECK (kind IN ('image', 'preview', 'wav', 'stems')),
    file_name VARCHAR(255) NOT NULL,
    object_key TEXT NOT NULL UNIQUE,
    final_object_key TEXT NOT NULL UNIQUE,
    declared_content_type VARCHAR(120) NOT NULL,
    actual_content_type VARCHAR(120),
    expected_size BIGINT NOT NULL CHECK (expected_size > 0),
    actual_size BIGINT,
    etag TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (session_id, kind)
);

CREATE INDEX idx_spec_upload_assets_session
    ON spec_upload_assets (session_id);

CREATE TABLE spec_processing_jobs (
    id UUID PRIMARY KEY,
    spec_id UUID NOT NULL UNIQUE REFERENCES specs(id) ON DELETE CASCADE,
    session_id UUID NOT NULL UNIQUE REFERENCES spec_upload_sessions(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'processing', 'completed', 'failed')),
    worker_id VARCHAR(120),
    locked_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_spec_processing_jobs_claim
    ON spec_processing_jobs (status, created_at)
    WHERE status IN ('queued', 'processing');
