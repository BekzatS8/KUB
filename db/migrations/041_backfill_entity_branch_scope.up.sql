-- =============================================================================
-- Migration 041: backfill branch_id for entities that still have NULL
-- =============================================================================
-- Idempotent throughout: every UPDATE is guarded by WHERE branch_id IS NULL.
-- Mirrors the logic already applied in migrations 027, 030, 031 — safe to
-- re-run; records that already have branch_id are not touched.
--
-- Fallback strategy for unrecoverable records:
--   After owner/creator cascade, records that still have NULL branch_id have
--   no resolvable source. They are assigned the lowest-id active branch so
--   that branch-scoped roles can see them. If no active branch exists the
--   fallback block skips with RAISE NOTICE — records remain NULL until a
--   branch is created.
--
-- Intentionally NOT backfilled (left NULL):
--   chats           — creator-based recovery runs (step 6), but no default
--                     fallback; chats created by system/no-branch users stay
--                     NULL and are accessible to admin/management (ScopeAll).
--   telephony_calls — branch_id is NULL when no manager is assigned; that is
--                     a legitimate runtime state (external inbound call before
--                     routing). Forcing a default branch would misattribute
--                     the call to the wrong office.
-- =============================================================================

BEGIN;

-- Step 1: leads ← owner.branch_id
UPDATE leads l
SET branch_id = u.branch_id
FROM users u
WHERE l.owner_id = u.id
  AND l.branch_id IS NULL
  AND u.branch_id IS NOT NULL;

-- Step 2a: deals ← lead.branch_id  (primary source: inherited pipeline)
UPDATE deals d
SET branch_id = l.branch_id
FROM leads l
WHERE d.lead_id = l.id
  AND d.branch_id IS NULL
  AND l.branch_id IS NOT NULL;

-- Step 2b: deals ← owner.branch_id  (fallback when lead also had NULL)
UPDATE deals d
SET branch_id = u.branch_id
FROM users u
WHERE d.owner_id = u.id
  AND d.branch_id IS NULL
  AND u.branch_id IS NOT NULL;

-- Step 3: clients ← owner.branch_id
UPDATE clients c
SET branch_id = u.branch_id
FROM users u
WHERE c.owner_id = u.id
  AND c.branch_id IS NULL
  AND u.branch_id IS NOT NULL;

-- Step 4: tasks ← creator.branch_id
UPDATE tasks t
SET branch_id = u.branch_id
FROM users u
WHERE t.creator_id = u.id
  AND t.branch_id IS NULL
  AND u.branch_id IS NOT NULL;

-- Step 5: documents ← deal.branch_id
UPDATE documents dc
SET branch_id = d.branch_id
FROM deals d
WHERE dc.deal_id = d.id
  AND dc.branch_id IS NULL
  AND d.branch_id IS NOT NULL;

-- Step 6: chats ← creator.branch_id  (no default fallback — see header note)
UPDATE chats c
SET branch_id = u.branch_id
FROM users u
WHERE c.creator_id = u.id
  AND c.branch_id IS NULL
  AND u.branch_id IS NOT NULL;

-- Step 7: default-branch fallback for leads/deals/clients/tasks/documents
--         that remain NULL after cascade recovery.
--         telephony_calls and chats are intentionally excluded.
DO $$
DECLARE
    default_branch_id INT;
    cnt_leads    INT;
    cnt_deals    INT;
    cnt_clients  INT;
    cnt_tasks    INT;
    cnt_docs     INT;
BEGIN
    SELECT id INTO default_branch_id
    FROM branches
    WHERE is_active = TRUE
    ORDER BY id
    LIMIT 1;

    IF default_branch_id IS NULL THEN
        RAISE NOTICE
            'Migration 041: no active branch found — skipping fallback for unrecoverable records. '
            'Run again after creating a branch.';
        RETURN;
    END IF;

    UPDATE leads      SET branch_id = default_branch_id WHERE branch_id IS NULL;
    GET DIAGNOSTICS cnt_leads = ROW_COUNT;

    UPDATE deals      SET branch_id = default_branch_id WHERE branch_id IS NULL;
    GET DIAGNOSTICS cnt_deals = ROW_COUNT;

    UPDATE clients    SET branch_id = default_branch_id WHERE branch_id IS NULL;
    GET DIAGNOSTICS cnt_clients = ROW_COUNT;

    UPDATE tasks      SET branch_id = default_branch_id WHERE branch_id IS NULL;
    GET DIAGNOSTICS cnt_tasks = ROW_COUNT;

    UPDATE documents  SET branch_id = default_branch_id WHERE branch_id IS NULL;
    GET DIAGNOSTICS cnt_docs = ROW_COUNT;

    RAISE NOTICE
        'Migration 041 fallback: assigned branch_id=% to % leads, % deals, % clients, % tasks, % documents.',
        default_branch_id, cnt_leads, cnt_deals, cnt_clients, cnt_tasks, cnt_docs;
END $$;

COMMIT;
