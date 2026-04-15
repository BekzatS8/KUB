BEGIN;

DROP INDEX IF EXISTS chats_company_idx;
DROP INDEX IF EXISTS tasks_company_idx;
DROP INDEX IF EXISTS documents_company_idx;
DROP INDEX IF EXISTS deals_company_idx;
DROP INDEX IF EXISTS leads_company_idx;

ALTER TABLE chats DROP COLUMN IF EXISTS company_id;
ALTER TABLE tasks DROP COLUMN IF EXISTS company_id;
ALTER TABLE documents DROP COLUMN IF EXISTS company_id;
ALTER TABLE deals DROP COLUMN IF EXISTS company_id;
ALTER TABLE leads DROP COLUMN IF EXISTS company_id;

DROP TABLE IF EXISTS company_backfill_unresolved;

COMMIT;
