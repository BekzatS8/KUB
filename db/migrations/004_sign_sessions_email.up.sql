-- ===================== SIGN SESSIONS (EMAIL FLOW) =====================

BEGIN;

ALTER TABLE sign_sessions
    ADD COLUMN IF NOT EXISTS signer_email TEXT,
    ADD COLUMN IF NOT EXISTS doc_hash TEXT;

ALTER TABLE sign_sessions
    ALTER COLUMN phone_e164 DROP NOT NULL,
    ALTER COLUMN code_hash DROP NOT NULL;

CREATE INDEX IF NOT EXISTS sign_sessions_document_email_idx ON sign_sessions(document_id, signer_email);

COMMIT;
