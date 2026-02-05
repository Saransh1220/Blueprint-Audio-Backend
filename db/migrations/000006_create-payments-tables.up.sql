-- Orders table: Represents purchase intent before payment
-- An order is created when user initiates checkout
CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    spec_id UUID NOT NULL REFERENCES specs(id) ON DELETE RESTRICT,
    
    -- License details
    license_type VARCHAR(50) NOT NULL CHECK (license_type IN ('Basic', 'Premium', 'Trackout', 'Unlimited')),
    
    -- Pricing
    amount INTEGER NOT NULL CHECK (amount > 0), -- Store in smallest currency unit (paise for INR)
    currency VARCHAR(3) NOT NULL DEFAULT 'INR',
    
    -- Razorpay integration
    razorpay_order_id VARCHAR(255) UNIQUE, -- Razorpay's order ID
    
    -- Order status lifecycle
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (
        status IN ('pending', 'processing', 'paid', 'failed', 'cancelled', 'refunded')
    ),
    
    -- Metadata
    notes JSONB DEFAULT '{}', -- Additional order metadata (user notes, promo codes, etc.)
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() + INTERVAL '30 minutes' -- Order expiry
);


-- Payments table: Represents actual payment transactions
-- A payment is created after successful payment confirmation
CREATE TABLE payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    
    -- Razorpay payment details
    razorpay_payment_id VARCHAR(255) NOT NULL UNIQUE,
    razorpay_signature VARCHAR(512) NOT NULL, -- HMAC signature for verification
    
    -- Payment details
    amount INTEGER NOT NULL CHECK (amount > 0),
    currency VARCHAR(3) NOT NULL DEFAULT 'INR',
    
    -- Payment status
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (
        status IN ('pending', 'captured', 'failed', 'refunded')
    ),
    
    -- Payment method details
    method VARCHAR(50), -- card, netbanking, upi, wallet, etc.
    bank VARCHAR(100), -- Bank name if applicable
    wallet VARCHAR(50), -- Wallet provider if applicable
    vpa VARCHAR(255), -- UPI VPA if applicable
    card_network VARCHAR(50), -- Visa, Mastercard, Rupay, etc.
    card_last4 VARCHAR(4), -- Last 4 digits of card
    
    -- Additional metadata
    email VARCHAR(255), -- User email at time of payment
    contact VARCHAR(20), -- User contact at time of payment
    error_code VARCHAR(100), -- Razorpay error code if failed
    error_description TEXT, -- Error message if failed
    
    -- Timestamps
    captured_at TIMESTAMP WITH TIME ZONE, -- When payment was captured
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Licenses table
CREATE TABLE licenses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    spec_id UUID NOT NULL REFERENCES specs(id) ON DELETE RESTRICT,
    license_option_id UUID NOT NULL REFERENCES license_options(id) ON DELETE RESTRICT,
    
    license_type VARCHAR(50) NOT NULL,
    purchase_price INTEGER NOT NULL,
    license_key VARCHAR(255) UNIQUE NOT NULL,
    
    is_active BOOLEAN DEFAULT true,
    is_revoked BOOLEAN DEFAULT false,
    revoked_reason TEXT,
    revoked_at TIMESTAMP WITH TIME ZONE,
    
    downloads_count INTEGER DEFAULT 0,
    last_downloaded_at TIMESTAMP WITH TIME ZONE,
    
    issued_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_created_at ON orders(created_at DESC);

CREATE INDEX idx_payments_order_id ON payments(order_id);
CREATE INDEX idx_payments_razorpay_payment_id ON payments(razorpay_payment_id);
CREATE INDEX idx_payments_status ON payments(status);

CREATE INDEX idx_licenses_user_id ON licenses(user_id);
CREATE INDEX idx_licenses_order_id ON licenses(order_id);
CREATE INDEX idx_licenses_spec_id ON licenses(spec_id);
CREATE INDEX idx_licenses_is_active ON licenses(is_active) WHERE is_active = true;
CREATE INDEX idx_licenses_license_key ON licenses(license_key);

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;


-- Triggers
CREATE TRIGGER update_orders_updated_at
    BEFORE UPDATE ON orders
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_payments_updated_at
    BEFORE UPDATE ON payments
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_licenses_updated_at
    BEFORE UPDATE ON licenses
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();