-- 071_document_template_departments.up.sql
-- Шаблоны документов по отделам (обратная связь 14.07.2026).
--
-- Сами шаблоны живут в коде (internal/services/document_registry.go): docx-файл
-- плюс описание полей, которые подставляются из клиента и сделки. А вот кому
-- какой шаблон нужен — это бизнес-настройка, а не код: админ раскидывает
-- шаблоны по отделам в Настройках, без деплоя. Отдел видит свои шаблоны и
-- генерирует по ним документы клиентов (scope 'deal').
--
-- Шаблон может принадлежать нескольким отделам: расписку о возврате оформляют
-- и продажи, и юристы.
-- This migration is re-applied on every deploy, so every statement is idempotent.

CREATE TABLE IF NOT EXISTS document_template_departments (
    doc_type   TEXT NOT NULL,
    scope      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (doc_type, scope)
);

-- те же коды, что и у documents.scope, но без 'deal': 'deal' — это уже
-- сгенерированный документ клиента, а не отдел
ALTER TABLE document_template_departments DROP CONSTRAINT IF EXISTS document_template_departments_scope_chk;
ALTER TABLE document_template_departments ADD CONSTRAINT document_template_departments_scope_chk CHECK (
    scope IN ('hr', 'legal', 'sales', 'visa', 'partner', 'quality_control', 'management')
);

CREATE INDEX IF NOT EXISTS document_template_departments_scope_idx ON document_template_departments(scope);

-- Стартовая раскладка, чтобы отделы не открылись пустыми.
--
-- Заполняется ТОЛЬКО когда таблица пуста (WHERE NOT EXISTS), а не через
-- ON CONFLICT DO NOTHING: миграции прогоняются на каждом деплое, и DO NOTHING
-- воскрешал бы связи, которые админ намеренно убрал.
-- doc_type должны существовать в services.documentTypeRegistry, иначе шаблон
-- не покажется ни в одном отделе. Список синхронизирован с редакцией
-- шаблонов от 16.07.2026 (16 типов).
INSERT INTO document_template_departments (doc_type, scope)
SELECT seed.doc_type, seed.scope
FROM (VALUES
    -- продажи: договоры и допсоглашения
    ('contract_paid_50_50_ru',        'sales'),
    ('contract_paid_full_ru',         'sales'),
    ('contract_ukaby_30_35_35',       'sales'),
    ('contract_language_courses',     'sales'),
    ('addendum_c01_extension',        'sales'),
    ('addendum_k01_korea',            'sales'),
    -- визовый отдел
    ('cancel_appointment',            'visa'),
    ('documents_handover_act',        'visa'),
    ('power_of_attorney_application', 'visa'),
    -- юристы: расторжения, возвраты, приостановки
    ('termination_transfer',          'legal'),
    ('termination_waiver',            'legal'),
    ('refund_application',            'legal'),
    ('receipt_refund_full',           'legal'),
    ('receipt_refund_partial',        'legal'),
    ('pause_application',             'legal'),
    -- руководство
    ('avr_kub_group',                 'management')
) AS seed(doc_type, scope)
WHERE NOT EXISTS (SELECT 1 FROM document_template_departments);
