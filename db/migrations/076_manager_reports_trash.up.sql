-- 076_manager_reports_trash.up.sql
-- Корзина для отчётов сотрудников (обратная связь заказчика 23.07.2026): как у
-- сделок/лидов/документов/задач (ТЗ 04.07.2026, п.7.1). Удаление отчёта — мягкое:
-- он уходит в корзину, откуда сотрудник (свой отчёт) или админ (любой) может
-- восстановить его или удалить окончательно.
-- This migration is re-applied on every deploy, so every statement is idempotent.

ALTER TABLE manager_reports ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE manager_reports ADD COLUMN IF NOT EXISTS deleted_by BIGINT;

CREATE INDEX IF NOT EXISTS manager_reports_deleted_at_idx ON manager_reports(deleted_at);
