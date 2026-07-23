-- 075_tasks_trash.up.sql
-- Корзина для задач (обратная связь заказчика 23.07.2026): у задач уже есть
-- архив (is_archived), добавляем мягкое удаление в корзину — как у сделок/
-- лидов/документов (ТЗ 04.07.2026, п.7.1). Удалённая задача уходит в корзину,
-- откуда её можно восстановить или удалить окончательно.
-- This migration is re-applied on every deploy, so every statement is idempotent.

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS deleted_by BIGINT;

CREATE INDEX IF NOT EXISTS tasks_deleted_at_idx ON tasks(deleted_at);
