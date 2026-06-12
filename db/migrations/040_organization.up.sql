BEGIN;

-- =============================================================================
-- Migration 040: organization (singleton — owner of this CRM instance)
--
-- A single row (id=1) represents the organization that owns/operates the CRM.
-- Contact fields cover TZ #5 requirements (phone/email/social media).
-- Legacy users.company_name / users.bin_iin are NOT removed here.
-- =============================================================================

CREATE TABLE IF NOT EXISTS organization (
    id          SERIAL PRIMARY KEY,
    name        TEXT NOT NULL DEFAULT '',
    legal_name  TEXT,
    bin         TEXT,
    phone       TEXT,
    email       TEXT,
    address     TEXT,
    website     TEXT,
    whatsapp    TEXT,
    telegram    TEXT,
    instagram   TEXT,
    tiktok      TEXT,
    logo_url    TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Guarantee singleton: only one row ever exists (id=1).
-- Partial unique index: allows exactly one active row at id=1.
CREATE UNIQUE INDEX IF NOT EXISTS organization_singleton_idx ON organization ((id = 1)) WHERE (id = 1);

-- Insert default row idempotently.
INSERT INTO organization (id, name) VALUES (1, '')
ON CONFLICT (id) DO NOTHING;

COMMIT;
