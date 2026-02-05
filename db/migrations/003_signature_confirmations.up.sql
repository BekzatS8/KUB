-- ===================== SIGNATURE CONFIRMATIONS =====================

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS signature_confirmations (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id  INT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    user_id      INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel      VARCHAR(20) NOT NULL,
    status       VARCHAR(20) NOT NULL,
    otp_hash     TEXT,
    token_hash   TEXT,
    attempts     INT NOT NULL DEFAULT 0,
    expires_at   TIMESTAMPTZ NOT NULL,
    approved_at  TIMESTAMPTZ,
    rejected_at  TIMESTAMPTZ,
    meta         JSONB,
    CONSTRAINT signature_confirmations_channel_chk CHECK (channel IN ('email', 'telegram')),
    CONSTRAINT signature_confirmations_status_chk CHECK (
        status IN ('pending', 'approved', 'rejected', 'expired', 'cancelled')
    ),
    CONSTRAINT signature_confirmations_attempts_chk CHECK (attempts >= 0)
);

CREATE INDEX IF NOT EXISTS signature_confirmations_doc_user_status_idx
    ON signature_confirmations(document_id, user_id, status);
CREATE INDEX IF NOT EXISTS signature_confirmations_expires_idx
    ON signature_confirmations(expires_at);

COMMIT;
