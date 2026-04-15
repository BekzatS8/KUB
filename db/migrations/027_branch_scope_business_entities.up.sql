BEGIN;

ALTER TABLE leads ADD COLUMN IF NOT EXISTS branch_id INT REFERENCES branches(id);
ALTER TABLE deals ADD COLUMN IF NOT EXISTS branch_id INT REFERENCES branches(id);
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS branch_id INT REFERENCES branches(id);
ALTER TABLE documents ADD COLUMN IF NOT EXISTS branch_id INT REFERENCES branches(id);
ALTER TABLE chats ADD COLUMN IF NOT EXISTS branch_id INT REFERENCES branches(id);

CREATE INDEX IF NOT EXISTS leads_branch_id_idx ON leads(branch_id);
CREATE INDEX IF NOT EXISTS deals_branch_id_idx ON deals(branch_id);
CREATE INDEX IF NOT EXISTS tasks_branch_id_idx ON tasks(branch_id);
CREATE INDEX IF NOT EXISTS documents_branch_id_idx ON documents(branch_id);
CREATE INDEX IF NOT EXISTS chats_branch_id_idx ON chats(branch_id);

-- backfill leads/tasks from owner/creator branch
UPDATE leads l
SET branch_id = u.branch_id
FROM users u
WHERE l.owner_id = u.id
  AND l.branch_id IS NULL
  AND u.branch_id IS NOT NULL;

UPDATE tasks t
SET branch_id = u.branch_id
FROM users u
WHERE t.creator_id = u.id
  AND t.branch_id IS NULL
  AND u.branch_id IS NOT NULL;

-- deals inherit from lead first, then owner
UPDATE deals d
SET branch_id = l.branch_id
FROM leads l
WHERE d.lead_id = l.id
  AND d.branch_id IS NULL
  AND l.branch_id IS NOT NULL;

UPDATE deals d
SET branch_id = u.branch_id
FROM users u
WHERE d.owner_id = u.id
  AND d.branch_id IS NULL
  AND u.branch_id IS NOT NULL;

-- documents inherit from deal
UPDATE documents dc
SET branch_id = d.branch_id
FROM deals d
WHERE dc.deal_id = d.id
  AND dc.branch_id IS NULL
  AND d.branch_id IS NOT NULL;

-- chats inherit from creator branch
UPDATE chats c
SET branch_id = u.branch_id
FROM users u
WHERE c.creator_id = u.id
  AND c.branch_id IS NULL
  AND u.branch_id IS NOT NULL;

COMMIT;
