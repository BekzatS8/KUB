-- 068_manager_reports.up.sql
-- Отчёты менеджеров внутри CRM (ТЗ 04.07.2026, п.3).
--
-- Менеджеры вели ежедневные отчёты в Excel на Яндекс.Диске (дата, имя, телефон,
-- пометки) и скидывали их в WhatsApp руководству. Теперь у каждого сотрудника
-- своя редактируемая таблица прямо в CRM: content хранит колонки и строки в
-- JSON. Руководитель/КК открывают отчёт любого сотрудника на просмотр.
-- This migration is re-applied on every deploy, so every statement is idempotent.

CREATE TABLE IF NOT EXISTS manager_reports (
    id         SERIAL PRIMARY KEY,
    user_id    INT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    content    JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS manager_reports_updated_idx ON manager_reports(updated_at DESC);
