BEGIN;

-- Rollback migration 039: remove department_id from leads and deals.
-- Note: users.department_id backfill is NOT reversed (it was already valid
--   data that should have existed since migration 034).

DROP INDEX IF EXISTS deals_department_id_idx;
ALTER TABLE deals DROP COLUMN IF EXISTS department_id;

DROP INDEX IF EXISTS leads_department_id_idx;
ALTER TABLE leads DROP COLUMN IF EXISTS department_id;

COMMIT;
