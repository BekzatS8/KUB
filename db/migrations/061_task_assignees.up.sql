-- 061_task_assignees.up.sql
-- Support multiple assignees per task.
--
-- Historically a task had a single assignee (tasks.assignee_id). We keep that
-- column as the "primary" assignee for backward compatibility (notifications,
-- branch scoping, existing filters) and introduce a join table that holds the
-- full set of assignees. This migration is re-applied on every deploy, so every
-- statement is idempotent.

CREATE TABLE IF NOT EXISTS task_assignees (
    task_id INT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, user_id)
);

CREATE INDEX IF NOT EXISTS task_assignees_user_idx ON task_assignees(user_id);
CREATE INDEX IF NOT EXISTS task_assignees_task_idx ON task_assignees(task_id);

-- Backfill the join table from the legacy single-assignee column.
INSERT INTO task_assignees (task_id, user_id)
SELECT id, assignee_id
FROM tasks
WHERE assignee_id IS NOT NULL
ON CONFLICT DO NOTHING;
