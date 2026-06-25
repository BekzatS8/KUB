BEGIN;

-- Allow inbound leads (Wazzup/telephony) to have no assigned responsible.
-- Existing rows are unaffected (they already have non-NULL owner_id).
ALTER TABLE leads ALTER COLUMN owner_id DROP NOT NULL;

COMMIT;
