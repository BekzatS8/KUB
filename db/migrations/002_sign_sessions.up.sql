-- ===================== SIGN SESSIONS =====================

BEGIN;

CREATE TABLE IF NOT EXISTS sign_sessions (
    id               BIGSERIAL PRIMARY KEY,
    document_id      INT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    phone_e164       VARCHAR(20) NOT NULL,
    code_hash        TEXT NOT NULL,
    token_hash       CHAR(64) NOT NULL UNIQUE,
    expires_at       TIMESTAMPTZ NOT NULL,
    attempts         INT NOT NULL DEFAULT 0,
    status           VARCHAR(20) NOT NULL DEFAULT 'pending',
    verified_at      TIMESTAMPTZ,
    signed_at        TIMESTAMPTZ,
    signed_ip        TEXT,
    signed_user_agent TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT sign_sessions_attempts_chk CHECK (attempts >= 0),
    CONSTRAINT sign_sessions_status_chk CHECK (status IN ('pending', 'verified', 'signed', 'expired'))
);

CREATE INDEX IF NOT EXISTS sign_sessions_document_idx ON sign_sessions(document_id);
CREATE INDEX IF NOT EXISTS sign_sessions_expires_idx ON sign_sessions(expires_at);
CREATE INDEX IF NOT EXISTS sign_sessions_phone_idx ON sign_sessions(phone_e164);

COMMIT;
