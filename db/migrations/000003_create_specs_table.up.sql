-- 1. Main Specs Table
CREATE TABLE specs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    producer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL,
    category VARCHAR(20) NOT NULL CHECK (category IN ('beat', 'sample')),
    type VARCHAR(50) NOT NULL,       -- e.g., 'WAV', 'STEMS', 'PACK'
    bpm INTEGER NOT NULL CHECK (bpm >= 60 AND bpm <= 200),
    key VARCHAR(20) NOT NULL,
    base_price DECIMAL(10,2) NOT NULL CHECK (base_price >= 0),
    image_url VARCHAR(500) NOT NULL,
    
    -- Mandatory Audio Formats 
    preview_url VARCHAR(500) NOT NULL, -- MP3 for preview
    wav_url     VARCHAR(500),          -- Mandatory for beats
    stems_url   VARCHAR(500),          -- Mandatory for beats
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- 2. Link table for Many-to-Many relationship with Genres
CREATE TABLE spec_genres (
    spec_id UUID NOT NULL REFERENCES specs(id) ON DELETE CASCADE,
    genre_id UUID NOT NULL REFERENCES genres(id) ON DELETE CASCADE,
    PRIMARY KEY (spec_id, genre_id)
);

-- 3. Optimization: Index for producer lookups
CREATE INDEX idx_specs_producer_id ON specs(producer_id);