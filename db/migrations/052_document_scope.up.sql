ALTER TABLE documents ADD COLUMN scope VARCHAR(20) NOT NULL DEFAULT 'deal';
ALTER TABLE documents ADD COLUMN title TEXT;
ALTER TABLE documents ADD COLUMN description TEXT;
ALTER TABLE documents ADD COLUMN target_user_id INT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE documents ADD CONSTRAINT documents_scope_chk CHECK (scope IN ('deal','hr','legal'));
CREATE INDEX idx_documents_scope ON documents(scope);
