ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS is_hidden BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS audit_logs_hidden_idx ON audit_logs(is_hidden) WHERE is_hidden = TRUE;
