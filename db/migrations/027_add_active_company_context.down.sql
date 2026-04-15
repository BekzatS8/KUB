BEGIN;

DROP INDEX IF EXISTS users_active_company_idx;
ALTER TABLE users DROP COLUMN IF EXISTS active_company_id;

COMMIT;
