BEGIN;

DROP TABLE IF EXISTS wazzup_dedup;
DROP TABLE IF EXISTS wazzup_integrations;

DROP INDEX IF EXISTS wazzup_integrations_owner_user_uq;

DROP INDEX IF EXISTS leads_phone_idx;

ALTER TABLE IF EXISTS leads
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS phone;

COMMIT;
