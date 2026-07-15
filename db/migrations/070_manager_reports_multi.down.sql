-- 070_manager_reports_multi.down.sql
-- Возврат к «одному отчёту на сотрудника»: лишние отчёты удаляются,
-- остаётся последний обновлённый.
DELETE FROM manager_reports mr
WHERE mr.id NOT IN (
    SELECT DISTINCT ON (user_id) id
    FROM manager_reports
    ORDER BY user_id, updated_at DESC, id DESC
);

DROP INDEX IF EXISTS manager_reports_user_idx;

ALTER TABLE manager_reports DROP COLUMN IF EXISTS title;
ALTER TABLE manager_reports DROP COLUMN IF EXISTS created_at;

ALTER TABLE manager_reports ADD CONSTRAINT manager_reports_user_id_key UNIQUE (user_id);
