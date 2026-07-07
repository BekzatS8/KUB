-- 063_task_assignee_notifications.down.sql
DROP INDEX IF EXISTS task_assignees_notify_idx;
ALTER TABLE task_assignees DROP COLUMN IF EXISTS last_notified_at;
