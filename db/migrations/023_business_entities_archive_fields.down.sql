BEGIN;

DROP INDEX IF EXISTS tasks_archived_idx;
DROP INDEX IF EXISTS tasks_active_idx;
DROP INDEX IF EXISTS documents_archived_idx;
DROP INDEX IF EXISTS documents_active_idx;
DROP INDEX IF EXISTS clients_archived_idx;
DROP INDEX IF EXISTS clients_active_idx;
DROP INDEX IF EXISTS deals_archived_idx;
DROP INDEX IF EXISTS deals_active_idx;
DROP INDEX IF EXISTS leads_archived_idx;
DROP INDEX IF EXISTS leads_active_idx;

ALTER TABLE tasks
    DROP COLUMN IF EXISTS archive_reason,
    DROP COLUMN IF EXISTS archived_by,
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS is_archived;

ALTER TABLE documents
    DROP COLUMN IF EXISTS archive_reason,
    DROP COLUMN IF EXISTS archived_by,
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS is_archived;

ALTER TABLE clients
    DROP COLUMN IF EXISTS archive_reason,
    DROP COLUMN IF EXISTS archived_by,
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS is_archived;

ALTER TABLE deals
    DROP COLUMN IF EXISTS archive_reason,
    DROP COLUMN IF EXISTS archived_by,
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS is_archived;

ALTER TABLE leads
    DROP COLUMN IF EXISTS archive_reason,
    DROP COLUMN IF EXISTS archived_by,
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS is_archived;

COMMIT;
