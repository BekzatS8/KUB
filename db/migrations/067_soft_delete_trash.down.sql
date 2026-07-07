-- 067_soft_delete_trash.down.sql
DROP INDEX IF EXISTS leads_deleted_idx;
DROP INDEX IF EXISTS deals_deleted_idx;
DROP INDEX IF EXISTS documents_deleted_idx;
DROP INDEX IF EXISTS clients_deleted_idx;
ALTER TABLE leads     DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE leads     DROP COLUMN IF EXISTS deleted_by;
ALTER TABLE deals     DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE deals     DROP COLUMN IF EXISTS deleted_by;
ALTER TABLE documents DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE documents DROP COLUMN IF EXISTS deleted_by;
ALTER TABLE clients   DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE clients   DROP COLUMN IF EXISTS deleted_by;
