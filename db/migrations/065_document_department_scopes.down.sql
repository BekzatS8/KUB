-- 065_document_department_scopes.down.sql
ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_scope_chk;
ALTER TABLE documents ADD CONSTRAINT documents_scope_chk CHECK (scope IN ('deal','hr','legal'));
