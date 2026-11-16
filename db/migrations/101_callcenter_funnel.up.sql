-- 101_callcenter_funnel.up.sql
-- Замечания заказчика 13.11.2026: входящие лиды должны сначала попадать к
-- операторам колл-центра, а они передают их в отдел продаж.
--
-- crm_settings — настройки CRM «ключ → значение». inbound_to_callcenter = 'true'
-- включает маршрут «входящие → воронка колл-центра» (по умолчанию выключен:
-- без операторов входящие лиды некому разбирать).
-- Воронка «Обработка - Колл-центр» (callcenter_default) создаётся неактивной и
-- включается вместе с настройкой. Всё идемпотентно: миграции идут при каждом деплое.

CREATE TABLE IF NOT EXISTS crm_settings (
    key        VARCHAR(100) PRIMARY KEY,
    value      TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by INT REFERENCES users(id) ON DELETE SET NULL
);

INSERT INTO departments (name, code)
SELECT 'Колл-центр', 'operator'
WHERE NOT EXISTS (SELECT 1 FROM departments WHERE code = 'operator');

INSERT INTO funnels (name, code, department_id, is_active, sort_order)
SELECT 'Обработка - Колл-центр', 'callcenter_default', d.id, FALSE,
       (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM funnels)  -- последней: доска продаж остаётся первой
FROM departments d
WHERE d.code = 'operator'
ON CONFLICT (code) DO NOTHING;

INSERT INTO funnel_stages (funnel_id, name, code, color, type, position, probability)
SELECT f.id, s.name, s.code, s.color, s.type, s.position, s.probability
FROM funnels f
CROSS JOIN (VALUES
    ('Новая заявка', 'new',         '#94a3b8', 'regular', 10, 10),
    ('В работе',     'in_progress', '#3b82f6', 'regular', 20, 30),
    ('Отказ',        'lost',        '#ef4444', 'lost',    30, 0)
) AS s(name, code, color, type, position, probability)
WHERE f.code = 'callcenter_default'
ON CONFLICT (funnel_id, code) DO NOTHING;
