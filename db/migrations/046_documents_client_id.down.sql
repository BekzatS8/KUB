-- 046 down: Remove client_id from documents
ALTER TABLE documents DROP COLUMN IF EXISTS client_id;
DROP INDEX IF EXISTS idx_documents_client_id;
