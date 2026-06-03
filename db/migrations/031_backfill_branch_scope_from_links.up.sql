-- Backfill branch scope for records that can safely inherit it from linked data.
UPDATE documents d
SET branch_id = dl.branch_id
FROM deals dl
WHERE d.branch_id IS NULL
  AND d.deal_id = dl.id
  AND dl.branch_id IS NOT NULL;

UPDATE tasks t
SET branch_id = u.branch_id
FROM users u
WHERE t.branch_id IS NULL
  AND t.creator_id = u.id
  AND u.branch_id IS NOT NULL;

UPDATE chats c
SET branch_id = u.branch_id
FROM users u
WHERE c.branch_id IS NULL
  AND c.creator_id = u.id
  AND u.branch_id IS NOT NULL;
