-- ===================== SIGNATURE CONFIRMATIONS CREATED_AT =====================

BEGIN;

ALTER TABLE signature_confirmations
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

COMMIT;
