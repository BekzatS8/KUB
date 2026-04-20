BEGIN;

ALTER TABLE clients
    ADD COLUMN IF NOT EXISTS branch_id INT REFERENCES branches(id);

CREATE INDEX IF NOT EXISTS clients_branch_id_idx ON clients(branch_id);

-- Backfill strategy:
-- 1) Prefer owner's branch when owner exists and has users.branch_id.
-- 2) Keep NULL when owner is missing or branch cannot be resolved.
--    NULL remains inaccessible for branch-scoped roles and can be repaired manually.
UPDATE clients c
SET branch_id = u.branch_id
FROM users u
WHERE c.owner_id = u.id
  AND c.branch_id IS NULL
  AND u.branch_id IS NOT NULL;

COMMIT;
