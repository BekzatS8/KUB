-- ========== SIGNATURE CONFIRMATIONS CHANNEL REMOVE SMS ==========

BEGIN;

ALTER TABLE signature_confirmations
    DROP CONSTRAINT IF EXISTS signature_confirmations_channel_chk;

ALTER TABLE signature_confirmations
    ADD CONSTRAINT signature_confirmations_channel_chk
        CHECK (channel IN ('email', 'telegram'));

COMMIT;
