-- 046: Add client_id to documents table for direct client-document relationship
BEGIN;

-- Add nullable client_id column
ALTER TABLE documents ADD COLUMN IF NOT EXISTS client_id INT REFERENCES clients(id);

-- Index for fast lookups by client
CREATE INDEX IF NOT EXISTS idx_documents_client_id ON documents(client_id);

-- Backfill client_id from existing deals
UPDATE documents d
SET client_id = deal.client_id
FROM deals deal
WHERE d.deal_id = deal.id
  AND d.client_id IS NULL;

COMMIT;
