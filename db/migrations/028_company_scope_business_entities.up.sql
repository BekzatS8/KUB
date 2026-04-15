BEGIN;

ALTER TABLE leads ADD COLUMN IF NOT EXISTS company_id INT REFERENCES companies(id);
ALTER TABLE deals ADD COLUMN IF NOT EXISTS company_id INT REFERENCES companies(id);
ALTER TABLE documents ADD COLUMN IF NOT EXISTS company_id INT REFERENCES companies(id);
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS company_id INT REFERENCES companies(id);
ALTER TABLE chats ADD COLUMN IF NOT EXISTS company_id INT REFERENCES companies(id);

CREATE INDEX IF NOT EXISTS leads_company_idx ON leads(company_id);
CREATE INDEX IF NOT EXISTS deals_company_idx ON deals(company_id);
CREATE INDEX IF NOT EXISTS documents_company_idx ON documents(company_id);
CREATE INDEX IF NOT EXISTS tasks_company_idx ON tasks(company_id);
CREATE INDEX IF NOT EXISTS chats_company_idx ON chats(company_id);

CREATE TABLE IF NOT EXISTS company_backfill_unresolved (
    id BIGSERIAL PRIMARY KEY,
    entity_type VARCHAR(50) NOT NULL,
    entity_id BIGINT NOT NULL,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(entity_type, entity_id)
);

-- leads: owner -> users.active_company_id
UPDATE leads l
SET company_id = u.active_company_id
FROM users u
WHERE l.owner_id = u.id
  AND l.company_id IS NULL
  AND u.active_company_id IS NOT NULL;

-- deals: from lead.company_id, fallback owner.active_company_id
UPDATE deals d
SET company_id = l.company_id
FROM leads l
WHERE d.lead_id = l.id
  AND d.company_id IS NULL
  AND l.company_id IS NOT NULL;

UPDATE deals d
SET company_id = u.active_company_id
FROM users u
WHERE d.owner_id = u.id
  AND d.company_id IS NULL
  AND u.active_company_id IS NOT NULL;

-- documents: from deals.company_id
UPDATE documents doc
SET company_id = d.company_id
FROM deals d
WHERE doc.deal_id = d.id
  AND doc.company_id IS NULL
  AND d.company_id IS NOT NULL;

-- tasks: creator.active_company_id fallback assignee.active_company_id
UPDATE tasks t
SET company_id = u.active_company_id
FROM users u
WHERE t.creator_id = u.id
  AND t.company_id IS NULL
  AND u.active_company_id IS NOT NULL;

UPDATE tasks t
SET company_id = u.active_company_id
FROM users u
WHERE t.assignee_id = u.id
  AND t.company_id IS NULL
  AND u.active_company_id IS NOT NULL;

-- chats: creator.active_company_id
UPDATE chats c
SET company_id = u.active_company_id
FROM users u
WHERE c.creator_id = u.id
  AND c.company_id IS NULL
  AND u.active_company_id IS NOT NULL;

-- unresolved rows report
INSERT INTO company_backfill_unresolved(entity_type, entity_id, reason)
SELECT 'leads', l.id, 'Unable to derive company_id from owner active_company_id'
FROM leads l
WHERE l.company_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO company_backfill_unresolved(entity_type, entity_id, reason)
SELECT 'deals', d.id, 'Unable to derive company_id from lead/owner'
FROM deals d
WHERE d.company_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO company_backfill_unresolved(entity_type, entity_id, reason)
SELECT 'documents', doc.id, 'Unable to derive company_id from linked deal'
FROM documents doc
WHERE doc.company_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO company_backfill_unresolved(entity_type, entity_id, reason)
SELECT 'tasks', t.id, 'Unable to derive company_id from creator/assignee'
FROM tasks t
WHERE t.company_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO company_backfill_unresolved(entity_type, entity_id, reason)
SELECT 'chats', c.id, 'Unable to derive company_id from creator'
FROM chats c
WHERE c.company_id IS NULL
ON CONFLICT DO NOTHING;

COMMIT;
