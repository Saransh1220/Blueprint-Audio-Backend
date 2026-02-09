CREATE TABLE analytics_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    spec_id UUID NOT NULL REFERENCES specs(id) ON DELETE CASCADE,
    event_type VARCHAR(50) NOT NULL, -- 'play', 'download', 'favorite'
    user_id UUID, -- Optional, if logged in
    meta JSONB, -- For extra data like 'source', 'device', etc.
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_analytics_events_spec_id ON analytics_events(spec_id);
CREATE INDEX idx_analytics_events_created_at ON analytics_events(created_at);
CREATE INDEX idx_analytics_events_type ON analytics_events(event_type);

-- Backfill data: For every spec with play_count > 0, insert rows
-- spread randomly over the last 30 days
DO $$
DECLARE
    r RECORD;
    i INT;
BEGIN
    FOR r IN SELECT spec_id, play_count FROM spec_analytics WHERE play_count > 0 LOOP
        FOR i IN 1..r.play_count LOOP
            INSERT INTO analytics_events (spec_id, event_type, created_at)
            VALUES (r.spec_id, 'play', NOW() - (random() * interval '30 days'));
        END LOOP;
    END LOOP;
END $$;
