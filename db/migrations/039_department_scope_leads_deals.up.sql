BEGIN;

-- =============================================================================
-- Migration 039: department_id on leads and deals (department-scope Фаза 3b-2)
--
-- Idempotent throughout (IF NOT EXISTS, WHERE IS NULL guards on backfill).
-- All funnels already carry department_id (NOT NULL) from migration 034.
-- =============================================================================

-- Step 1: Backfill users.department_id from role (for users created before
--   migration 034 who have department_id = NULL).
--   Role codes and department codes are identical by canonical design, so a
--   JOIN on code resolves the mapping without hardcoded IDs.
UPDATE users u
SET department_id = d.id
FROM roles r
JOIN departments d ON d.code = r.code
WHERE u.role_id = r.id
  AND u.department_id IS NULL
  AND d.is_active = TRUE;

-- Step 2: Add department_id to leads (idempotent).
ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS department_id INT REFERENCES departments(id);

CREATE INDEX IF NOT EXISTS leads_department_id_idx ON leads(department_id);

-- Step 3: Backfill leads.department_id.
--   Primary source: funnels.department_id via leads.funnel_id.
--   Fallback:       owner user's department_id.
--   Orphaned rows (both NULL) keep department_id = NULL (soft fallback in scope).
UPDATE leads l
SET department_id = COALESCE(
    (SELECT f.department_id FROM funnels f WHERE f.id = l.funnel_id),
    (SELECT u.department_id FROM users u WHERE u.id = l.owner_id)
)
WHERE l.department_id IS NULL;

-- Step 4: Add department_id to deals (idempotent).
ALTER TABLE deals
    ADD COLUMN IF NOT EXISTS department_id INT REFERENCES departments(id);

CREATE INDEX IF NOT EXISTS deals_department_id_idx ON deals(department_id);

-- Step 5: Backfill deals.department_id.
--   Primary source: funnels.department_id via the lead's funnel_id.
--   Fallback:       deal owner's department_id.
UPDATE deals d
SET department_id = COALESCE(
    (SELECT f.department_id FROM funnels f
     JOIN leads l ON l.funnel_id = f.id
     WHERE l.id = d.lead_id
     LIMIT 1),
    (SELECT u.department_id FROM users u WHERE u.id = d.owner_id)
)
WHERE d.department_id IS NULL;

COMMIT;
