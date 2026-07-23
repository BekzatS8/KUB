-- 075_tasks_trash.down.sql
DROP INDEX IF EXISTS tasks_deleted_at_idx;
ALTER TABLE tasks DROP COLUMN IF EXISTS deleted_by;
ALTER TABLE tasks DROP COLUMN IF EXISTS deleted_at;
