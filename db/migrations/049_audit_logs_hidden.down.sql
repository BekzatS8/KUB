DROP INDEX IF EXISTS audit_logs_hidden_idx;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS is_hidden;
