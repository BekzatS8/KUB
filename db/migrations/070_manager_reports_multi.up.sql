-- 070_manager_reports_multi.up.sql
-- Несколько именованных отчётов у одного сотрудника.
--
-- В 068 на сотрудника приходилась ровно одна таблица (user_id UNIQUE). Теперь
-- менеджер заводит отдельный отчёт под каждую задачу и даёт ему имя, а
-- руководитель/КК выбирают, какой именно отчёт сотрудника открыть.
-- This migration is re-applied on every deploy, so every statement is idempotent.

ALTER TABLE manager_reports ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT 'Основной отчёт';
ALTER TABLE manager_reports ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- снимаем «один отчёт на сотрудника»; индекс, созданный вместе с constraint,
-- уходит вместе с ним — второй DROP страхует случай ручного индекса.
ALTER TABLE manager_reports DROP CONSTRAINT IF EXISTS manager_reports_user_id_key;
DROP INDEX IF EXISTS manager_reports_user_id_key;

CREATE INDEX IF NOT EXISTS manager_reports_user_idx ON manager_reports(user_id, updated_at DESC);
