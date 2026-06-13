DROP INDEX IF EXISTS idx_documents_hidden_created_by;

ALTER TABLE documents
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS is_hidden;
