-- Этап отказа воронки. Когда карточка (лид/сделка) переходит на этап с
-- is_rejection = TRUE, она получает статус "отказ" (cancelled) — уходит из
-- активной воронки и попадает в «Отказники» (обратная связь заказчика
-- 30.07.2026). Отдельно от auto_archive: auto_archive → Архив, is_rejection →
-- Отказники. Миграции идемпотентны, значения не бэкфиллим.
BEGIN;

ALTER TABLE IF EXISTS funnel_stages
    ADD COLUMN IF NOT EXISTS is_rejection BOOLEAN NOT NULL DEFAULT FALSE;

COMMIT;
