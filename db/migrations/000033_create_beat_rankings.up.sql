CREATE TABLE beat_rankings (
    section VARCHAR(40) NOT NULL,
    period VARCHAR(20) NOT NULL,
    spec_id UUID NOT NULL REFERENCES specs(id) ON DELETE CASCADE,
    rank INTEGER NOT NULL,
    score NUMERIC(12,4) NOT NULL,
    previous_rank INTEGER,
    metrics JSONB NOT NULL DEFAULT '{}',
    calculated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (section, period, spec_id)
);

CREATE INDEX idx_beat_rankings_lookup
    ON beat_rankings(section, period, rank);

