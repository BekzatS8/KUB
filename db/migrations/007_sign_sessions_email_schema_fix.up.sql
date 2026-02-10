-- ===================== SIGN SESSIONS EMAIL SCHEMA FIX =====================

BEGIN;

ALTER TABLE sign_sessions
    ADD COLUMN IF NOT EXISTS signer_email TEXT,
    ADD COLUMN IF NOT EXISTS doc_hash CHAR(64);

ALTER TABLE sign_sessions
    ALTER COLUMN doc_hash TYPE CHAR(64) USING CASE
        WHEN doc_hash IS NULL THEN NULL
        ELSE LEFT(doc_hash, 64)
    END;

ALTER TABLE sign_sessions
    ALTER COLUMN phone_e164 DROP NOT NULL,
    ALTER COLUMN code_hash DROP NOT NULL;

CREATE INDEX IF NOT EXISTS sign_sessions_signer_email_idx ON sign_sessions(signer_email);

COMMIT;
