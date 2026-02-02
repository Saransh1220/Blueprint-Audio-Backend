CREATE TABLE license_options (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    spec_id UUID NOT NULL REFERENCES specs(id) ON DELETE CASCADE,
    license_type VARCHAR(20) NOT NULL CHECK (license_type IN ('Basic', 'Premium', 'Trackout', 'Unlimited')),
    name VARCHAR(100) NOT NULL, -- e.g., 'MP3 Lease'
    price DECIMAL(10,2) NOT NULL CHECK (price >= 0),
    features TEXT[] NOT NULL,   -- Array of strings like ['2000 Streams', 'Non-exclusive']
    file_types TEXT[] NOT NULL, -- Array of strings like ['MP3', 'WAV']
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    -- Constraint: A spec can't have two 'Basic' licenses
    UNIQUE(spec_id, license_type)
);

CREATE INDEX idx_license_options_spec_id ON license_options(spec_id);