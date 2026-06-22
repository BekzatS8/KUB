ALTER TABLE documents ADD COLUMN IF NOT EXISTS scope VARCHAR(20) NOT NULL DEFAULT 'deal';
ALTER TABLE documents ADD COLUMN IF NOT EXISTS title TEXT;
ALTER TABLE documents ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE documents ADD COLUMN IF NOT EXISTS target_user_id INT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_scope_chk;
ALTER TABLE documents ADD CONSTRAINT documents_scope_chk CHECK (scope IN ('deal','hr','legal'));
CREATE INDEX IF NOT EXISTS idx_documents_scope ON documents(scope);
