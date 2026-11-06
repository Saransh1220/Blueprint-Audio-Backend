-- 000041_create_legal_documents_tables.up.sql
-- Versioned legal policies, buyer licenses, and acceptance auditing

CREATE TABLE IF NOT EXISTS legal_documents (
    key VARCHAR(64) PRIMARY KEY,
    audience VARCHAR(32) NOT NULL DEFAULT 'buyer',
    locale VARCHAR(16) NOT NULL DEFAULT 'en',
    title VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS legal_document_versions (
    id UUID PRIMARY KEY,
    document_key VARCHAR(64) NOT NULL REFERENCES legal_documents(key) ON DELETE CASCADE,
    display_version VARCHAR(32) NOT NULL,
    schema_version INT NOT NULL DEFAULT 1,
    title VARCHAR(255) NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    sections JSONB NOT NULL DEFAULT '[]'::jsonb,
    content_hash VARCHAR(64) NOT NULL,
    update_classification VARCHAR(32) NOT NULL DEFAULT 'material',
    status VARCHAR(32) NOT NULL DEFAULT 'published',
    published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    effective_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    superseded_at TIMESTAMPTZ,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_legal_doc_versions_key_status ON legal_document_versions(document_key, status, effective_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_legal_doc_versions_key_ver ON legal_document_versions(document_key, display_version);

CREATE TABLE IF NOT EXISTS legal_acceptances (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    document_key VARCHAR(64) NOT NULL REFERENCES legal_documents(key) ON DELETE CASCADE,
    version_id UUID NOT NULL REFERENCES legal_document_versions(id) ON DELETE CASCADE,
    content_hash VARCHAR(64) NOT NULL,
    action VARCHAR(64) NOT NULL,
    context VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_legal_acceptances_user ON legal_acceptances(user_id, document_key);

CREATE TABLE IF NOT EXISTS legal_publication_audits (
    id UUID PRIMARY KEY,
    document_key VARCHAR(64) NOT NULL,
    version_id UUID NOT NULL,
    action VARCHAR(64) NOT NULL,
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed initial documents
INSERT INTO legal_documents (key, audience, locale, title) VALUES
    ('terms', 'buyer', 'en', 'Terms of Use'),
    ('privacy', 'buyer', 'en', 'Privacy Policy'),
    ('licensing', 'buyer', 'en', 'Licensing Guide & Terms')
ON CONFLICT (key) DO NOTHING;

-- Seed version 1.0.0 for Terms of Use
INSERT INTO legal_document_versions (
    id, document_key, display_version, schema_version, title, summary,
    sections, content_hash, update_classification, status, published_at, effective_at
) VALUES (
    '11111111-1111-1111-1111-111111111111',
    'terms',
    '1.0.0',
    1,
    'Terms of Use',
    'Clear terms governing your account, beat licensing purchases, digital deliverables, and consumer rights on Waveyard.',
    '[
        {"id": "who-we-are", "order": 1, "title": "Who we are and what these terms cover", "content": "Waveyard is a curated personal music producer storefront operated for licensing beats and sound assets. These Terms govern your access to the store, account creation, and purchases.", "summary": "Waveyard is an independent beat storefront, not a third-party open marketplace."},
        {"id": "eligibility", "order": 2, "title": "Eligibility and agreement", "content": "You must be at least 18 years old and legally competent to enter into a binding contract. By registering or completing a purchase, you agree to these Terms.", "summary": "Adults-only purchasing; affirmative agreement required."},
        {"id": "account-security", "order": 3, "title": "Your account and security", "content": "You are responsible for keeping your credentials confidential. You must notify us promptly of any unauthorized account activity. Do not share accounts to evade license limits.", "summary": "Keep your credentials safe; one licensee per purchase."},
        {"id": "orders-and-payments", "order": 4, "title": "Orders, prices, and payments", "content": "Checkout displays the exact currency, price, and payment gateway. Digital delivery occurs immediately upon verified payment. We will never apply post-purchase price changes without consent.", "summary": "Transparent pricing in INR or USD with instant digital delivery."},
        {"id": "content-licenses", "order": 5, "title": "Your content licenses", "content": "Each beat purchase grants you the specific non-exclusive license rights stated in the selected tier agreement (Basic, Premium, Trackout, or Unlimited). Purchasing does not transfer copyright.", "summary": "Licenses grant designated usage rights, not copyright ownership."},
        {"id": "refunds-and-remedies", "order": 6, "title": "Refunds and remedies", "content": "If deliverables are corrupted, materially defective, or duplicate charges occur, contact support for a prompt resolution, replacement, or refund in compliance with consumer protection laws.", "summary": "Remedies available for defects, duplicates, or fulfillment failures."},
        {"id": "acceptable-use", "order": 7, "title": "Acceptable use and copyright protection", "content": "You may not reverse-engineer, scrape, redistribute raw stems, or register licensed beats into Content ID systems that block other legitimate licensees.", "summary": "No raw stem resale; no abusive Content ID registrations."}
    ]'::jsonb,
    'a1b2c3d4e5f60718293a4b5c6d7e8f901234567890abcdef1234567890abcdef',
    'material',
    'published',
    NOW(),
    NOW()
) ON CONFLICT (document_key, display_version) DO NOTHING;

-- Seed version 1.0.0 for Privacy Policy
INSERT INTO legal_document_versions (
    id, document_key, display_version, schema_version, title, summary,
    sections, content_hash, update_classification, status, published_at, effective_at
) VALUES (
    '22222222-2222-2222-2222-222222222222',
    'privacy',
    '1.0.0',
    1,
    'Privacy Policy',
    'Our transparent commitments regarding personal data collection, account security, payment processing, and your privacy rights.',
    '[
        {"id": "data-controller", "order": 1, "title": "Who handles your information", "content": "Waveyard is the data controller for personal data collected through waveyard.studio and associated storefront services.", "summary": "Direct operator accountability for your personal data."},
        {"id": "data-collected", "order": 2, "title": "Information we collect and why", "content": "We collect account details (email, username, password hashes), transaction records for order fulfillment, and technical diagnostic telemetry.", "summary": "Only data necessary for account access and order fulfillment is collected."},
        {"id": "payments-and-processors", "order": 3, "title": "Payment processors and third parties", "content": "Payment transactions are handled securely by PCI-compliant gateways (Razorpay, Dodo Payments). We never store full card numbers or sensitive banking credentials on our servers.", "summary": "Zero storage of credit card or bank credentials on our systems."},
        {"id": "cookies-and-storage", "order": 4, "title": "Cookies and browser storage", "content": "We use strictly necessary browser storage for active sessions, cart contents, and interface preferences. We do not sell your personal data or deploy cross-site tracking brokers.", "summary": "Strictly functional cookies; zero third-party data brokers."},
        {"id": "your-rights", "order": 5, "title": "Your rights and data requests", "content": "You have the right to access, rectify, or request deletion of your personal data, subject to statutory accounting and tax retention requirements.", "summary": "Right to access, export, or delete your account data at any time."}
    ]'::jsonb,
    'b2c3d4e5f6a10718293a4b5c6d7e8f901234567890abcdef1234567890abcdef',
    'material',
    'published',
    NOW(),
    NOW()
) ON CONFLICT (document_key, display_version) DO NOTHING;

-- Seed version 1.0.0 for Licensing Guide & Terms
INSERT INTO legal_document_versions (
    id, document_key, display_version, schema_version, title, summary,
    sections, content_hash, update_classification, status, published_at, effective_at
) VALUES (
    '33333333-3333-3333-3333-333333333333',
    'licensing',
    '1.0.0',
    1,
    'Licensing Guide & Terms',
    'Comprehensive explanation of license tiers, deliverable audio file formats, commercial stream limits, and permitted usage rights.',
    '[
        {"id": "license-overview", "order": 1, "title": "How beat licensing works", "content": "Every beat available on Waveyard comes with clear license tier options. Licensing grants you permission to record vocals and distribute your new master recording under explicit terms.", "summary": "Choose the right tier based on your release and distribution needs."},
        {"id": "tier-comparison", "order": 2, "title": "License tier comparison", "content": "We offer four transparent license tiers: Basic Lease (MP3, 50k streams), Premium Lease (MP3 + WAV, 250k streams), Trackout Lease (MP3 + WAV + Separated Track Stems, 1M streams), and Unlimited Lease (All files, unlimited streams).", "summary": "Structured rights scaling from demo releases to commercial broadcasting.", "table": {"headers": ["License Tier", "Audio Files", "Audio Streams", "Music Videos", "Radio Stations"], "rows": [["Basic Lease", "Tagged/Untagged MP3", "50,000", "1 Non-monetized", "0"], ["Premium Lease", "MP3 + 24-bit WAV", "250,000", "1 Monetized", "2 Stations"], ["Trackout Lease", "MP3 + WAV + Stems", "1,000,000", "Unlimited", "Unlimited"], ["Unlimited Lease", "All Formats + Stems", "Unlimited", "Unlimited", "Unlimited"]]}},
        {"id": "free-vs-paid", "order": 3, "title": "Free downloads vs commercial licenses", "content": "Free MP3 downloads are provided solely for non-commercial audition, writing demos, or personal evaluation. Commercial release on streaming platforms requires purchasing a license tier.", "summary": "Free downloads = non-commercial audition only."},
        {"id": "credit-requirements", "order": 4, "title": "Credit and attribution", "content": "Licensees must give production credit in the title, description, or metadata where customary (e.g. Prod. by Waveyard).", "summary": "Producer credit required on all released recordings."},
        {"id": "retained-proof", "order": 5, "title": "Your license agreement proof", "content": "Upon successful payment, an immutable agreement record is pinned to your order. You can view, print, or download your verified license agreement proof anytime in Studio Purchases.", "summary": "Permanent, downloadable contract proof available in your account."}
    ]'::jsonb,
    'c3d4e5f6a1b20718293a4b5c6d7e8f901234567890abcdef1234567890abcdef',
    'material',
    'published',
    NOW(),
    NOW()
) ON CONFLICT (document_key, display_version) DO NOTHING;
