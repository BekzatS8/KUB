-- 095_operator_role_lead_brief.up.sql
-- ТЗ «доработки CRM КУБ», п.3 и п.5.
--
-- 1) Роль «Оператор» (колл-центр, id=95): первичная обработка лидов и передача
--    в отдел продаж. Права роли задаются в коде (authz.baseRolePermissions);
--    строк в role_permissions намеренно не создаём — /permissions/me тогда
--    берёт права из кода, и источник правды один.
-- 2) Отдел «Колл-центр» — для учётных записей операторов.
-- 3) «Краткая информация» в карточке лида (ФИО, страна, тип визы, город,
--    возраст, комментарий) и отметка о передаче в отдел продаж.
-- 4) История лида: изменения полей, передача, возврат с причиной, объединение.
--
-- Всё идемпотентно: миграции прогоняются при каждом деплое.

INSERT INTO roles (id, name, description, code) VALUES
    (95, 'Оператор (колл-центр)', 'Оператор колл-центра: первичная обработка лидов', 'operator')
ON CONFLICT (id) DO UPDATE
    SET code = EXCLUDED.code,
        description = EXCLUDED.description;

-- Роли, созданные через интерфейс, получают id из последовательности — пусть
-- она идёт после 95, чтобы не столкнуться с оператором.
SELECT setval('roles_id_seq', GREATEST(95, COALESCE((SELECT MAX(id) FROM roles), 95)));

INSERT INTO departments (name, code, is_active) VALUES
    ('Колл-центр', 'call_center', TRUE)
ON CONFLICT (code) DO NOTHING;

ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS brief_full_name    TEXT,
    ADD COLUMN IF NOT EXISTS brief_country      TEXT,
    ADD COLUMN IF NOT EXISTS brief_visa_type    TEXT,
    ADD COLUMN IF NOT EXISTS brief_city         TEXT,
    ADD COLUMN IF NOT EXISTS brief_age          TEXT,
    ADD COLUMN IF NOT EXISTS brief_comment      TEXT,
    ADD COLUMN IF NOT EXISTS handed_to_sales_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS handed_to_sales_by INT REFERENCES users(id) ON DELETE SET NULL,
    -- лид объединён с другим (п.3.3): ссылка на основной лид
    ADD COLUMN IF NOT EXISTS merged_into_id     INT REFERENCES leads(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS leads_handed_to_sales_by_idx ON leads(handed_to_sales_by) WHERE handed_to_sales_by IS NOT NULL;

CREATE TABLE IF NOT EXISTS lead_history (
    id         BIGSERIAL PRIMARY KEY,
    lead_id    INT NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    user_id    INT REFERENCES users(id) ON DELETE SET NULL,
    -- brief_update | handed_to_sales | returned | merged | merged_from | stage_moved
    action     TEXT NOT NULL,
    field      TEXT,
    old_value  TEXT,
    new_value  TEXT,
    comment    TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS lead_history_lead_idx ON lead_history(lead_id, created_at DESC);
