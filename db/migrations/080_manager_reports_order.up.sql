-- 080_manager_reports_order.up.sql
-- Порядок вкладок «Мои отчёты» (обратная связь заказчика 17.09.2026):
-- сотрудник настраивает очерёдность сам, а последний открытый отчёт становится
-- первым. Раньше список жёстко сортировался по updated_at DESC, поэтому порядок
-- менялся от любого сохранения таблицы и настроить его было нельзя.
-- This migration is re-applied on every deploy, so every statement is idempotent.

ALTER TABLE manager_reports ADD COLUMN IF NOT EXISTS position INT;

-- Разовая проставка стартового порядка: как список выглядел до этой миграции
-- (последний изменённый — первым). Только для строк без позиции, поэтому
-- повторный прогон не перетирает уже настроенный порядок.
WITH ordered AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY updated_at DESC, id DESC) AS rn
    FROM manager_reports
    WHERE position IS NULL
)
UPDATE manager_reports mr
SET position = ordered.rn
FROM ordered
WHERE mr.id = ordered.id;

CREATE INDEX IF NOT EXISTS manager_reports_user_position_idx
    ON manager_reports(user_id, position);
