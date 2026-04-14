BEGIN;

ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS is_archived BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS archived_by INT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS archive_reason TEXT;

ALTER TABLE deals
    ADD COLUMN IF NOT EXISTS is_archived BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS archived_by INT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS archive_reason TEXT;

ALTER TABLE clients
    ADD COLUMN IF NOT EXISTS is_archived BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS archived_by INT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS archive_reason TEXT;

ALTER TABLE documents
    ADD COLUMN IF NOT EXISTS is_archived BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS archived_by INT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS archive_reason TEXT;

ALTER TABLE tasks
    ADD COLUMN IF NOT EXISTS is_archived BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS archived_by INT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS archive_reason TEXT;

CREATE INDEX IF NOT EXISTS leads_active_idx ON leads (created_at DESC) WHERE is_archived = FALSE;
CREATE INDEX IF NOT EXISTS leads_archived_idx ON leads (archived_at DESC) WHERE is_archived = TRUE;

CREATE INDEX IF NOT EXISTS deals_active_idx ON deals (created_at DESC) WHERE is_archived = FALSE;
CREATE INDEX IF NOT EXISTS deals_archived_idx ON deals (archived_at DESC) WHERE is_archived = TRUE;

CREATE INDEX IF NOT EXISTS clients_active_idx ON clients (created_at DESC) WHERE is_archived = FALSE;
CREATE INDEX IF NOT EXISTS clients_archived_idx ON clients (archived_at DESC) WHERE is_archived = TRUE;

CREATE INDEX IF NOT EXISTS documents_active_idx ON documents (created_at DESC) WHERE is_archived = FALSE;
CREATE INDEX IF NOT EXISTS documents_archived_idx ON documents (archived_at DESC) WHERE is_archived = TRUE;

CREATE INDEX IF NOT EXISTS tasks_active_idx ON tasks (updated_at DESC) WHERE is_archived = FALSE;
CREATE INDEX IF NOT EXISTS tasks_archived_idx ON tasks (archived_at DESC) WHERE is_archived = TRUE;

COMMIT;
