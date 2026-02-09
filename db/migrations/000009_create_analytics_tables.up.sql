-- Spec-level analytics aggregation table
CREATE TABLE spec_analytics (
    spec_id UUID PRIMARY KEY REFERENCES specs(id) ON DELETE CASCADE,
    
    -- Public metrics
    play_count INTEGER DEFAULT 0,
    favorite_count INTEGER DEFAULT 0,
    free_download_count INTEGER DEFAULT 0,
    
    -- Producer-only metrics (aggregated from other tables)
    total_purchase_count INTEGER DEFAULT 0,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- User favorites (many-to-many)
CREATE TABLE user_favorites (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    spec_id UUID NOT NULL REFERENCES specs(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (user_id, spec_id)
);

-- Indexes for performance
CREATE INDEX idx_spec_analytics_play_count ON spec_analytics(play_count DESC);
CREATE INDEX idx_spec_analytics_favorite_count ON spec_analytics(favorite_count DESC);
CREATE INDEX idx_user_favorites_user_id ON user_favorites(user_id);
CREATE INDEX idx_user_favorites_spec_id ON user_favorites(spec_id);

-- Trigger to update spec_analytics updated_at
CREATE TRIGGER update_spec_analytics_updated_at
    BEFORE UPDATE ON spec_analytics
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
