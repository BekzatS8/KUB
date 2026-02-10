-- ===================== SIGN SESSIONS EMAIL FIELDS BACKFILL (DOWN) =====================

BEGIN;

DROP INDEX IF EXISTS sign_sessions_signer_email_idx;

ALTER TABLE sign_sessions
    DROP COLUMN IF EXISTS signer_email,
    DROP COLUMN IF EXISTS doc_hash;

ALTER TABLE sign_sessions
    ALTER COLUMN phone_e164 SET NOT NULL,
    ALTER COLUMN code_hash SET NOT NULL;

COMMIT;
