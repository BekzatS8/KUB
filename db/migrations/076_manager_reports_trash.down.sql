-- 076_manager_reports_trash.down.sql
DROP INDEX IF EXISTS manager_reports_deleted_at_idx;
ALTER TABLE manager_reports DROP COLUMN IF EXISTS deleted_by;
ALTER TABLE manager_reports DROP COLUMN IF EXISTS deleted_at;
