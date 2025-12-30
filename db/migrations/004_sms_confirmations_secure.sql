ALTER TABLE sms_confirmations
    ADD COLUMN IF NOT EXISTS code_hash TEXT,
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_resend_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS resend_count INT NOT NULL DEFAULT 0;

UPDATE sms_confirmations
SET code_hash = COALESCE(code_hash, ''),
    expires_at = COALESCE(expires_at, sent_at + INTERVAL '5 minutes')
WHERE code_hash IS NULL OR expires_at IS NULL;

ALTER TABLE sms_confirmations
    ALTER COLUMN code_hash SET NOT NULL,
    ALTER COLUMN expires_at SET NOT NULL;

ALTER TABLE sms_confirmations
    DROP COLUMN IF EXISTS sms_code;

ALTER TABLE sms_confirmations
    ADD CONSTRAINT sms_confirmations_attempts_chk CHECK (attempts >= 0),
    ADD CONSTRAINT sms_confirmations_resend_count_chk CHECK (resend_count >= 0);
