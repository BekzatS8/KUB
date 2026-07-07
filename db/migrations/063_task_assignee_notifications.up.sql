-- 063_task_assignee_notifications.up.sql
-- Nagging task notifications (ТЗ 04.07.2026, п.4.1).
--
-- Every open task must nag its assignees: a centered popup + sound repeated
-- every hour until the task is done. The "last shown" mark is tracked per
-- assignee (a task can have several assignees, each gets their own nag cycle).
-- This migration is re-applied on every deploy, so every statement is idempotent.

ALTER TABLE task_assignees ADD COLUMN IF NOT EXISTS last_notified_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS task_assignees_notify_idx ON task_assignees(user_id, last_notified_at);
