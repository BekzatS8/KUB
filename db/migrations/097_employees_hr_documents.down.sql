-- 097_employees_hr_documents.down.sql
-- Отозванные документы возвращаем в черновики, иначе старое ограничение не встанет.
UPDATE documents SET status = 'draft' WHERE status = 'revoked';
ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_status_chk;
ALTER TABLE documents ADD CONSTRAINT documents_status_chk CHECK (
    status IN ('draft', 'under_review', 'approved', 'returned', 'signed', 'sent_for_signature')
);

DROP INDEX IF EXISTS documents_employee_idx;
ALTER TABLE documents
    DROP COLUMN IF EXISTS revoked_at,
    DROP COLUMN IF EXISTS sent_to,
    DROP COLUMN IF EXISTS sent_via,
    DROP COLUMN IF EXISTS sent_at,
    DROP COLUMN IF EXISTS hr_template_id,
    DROP COLUMN IF EXISTS employee_id;

DROP TABLE IF EXISTS hr_document_templates;
DROP TABLE IF EXISTS employees;
