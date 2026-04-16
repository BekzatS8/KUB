-- ========== SIGNATURE CONFIRMATIONS CHANNEL ADD SMS ==========

BEGIN;

ALTER TABLE signature_confirmations
    DROP CONSTRAINT IF EXISTS signature_confirmations_channel_chk;

ALTER TABLE signature_confirmations
    ADD CONSTRAINT signature_confirmations_channel_chk
        CHECK (channel IN ('email', 'telegram', 'sms'));

COMMIT;
