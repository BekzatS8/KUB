-- 097_employees_hr_documents.up.sql
-- ТЗ «доработки CRM КУБ», п.1–2: раздел «Сотрудники» и кадровые документы с
-- онлайн-подписанием. Идемпотентно (миграции прогоняются при каждом деплое).

-- 1) Сотрудники. Независимый от учётных записей CRM справочник (не у каждого
--    сотрудника есть вход в CRM); связь с пользователем — необязательная.
CREATE TABLE IF NOT EXISTS employees (
    id                 SERIAL PRIMARY KEY,
    full_name          TEXT NOT NULL,
    birth_date         DATE NOT NULL,
    iin                VARCHAR(12) NOT NULL,
    address            TEXT NOT NULL,
    phone              VARCHAR(20) NOT NULL,
    email              TEXT,
    emergency_name     TEXT NOT NULL,
    emergency_relation TEXT NOT NULL,
    emergency_phone    VARCHAR(20) NOT NULL,
    position           TEXT NOT NULL,
    salary             NUMERIC(14, 2) NOT NULL,
    hired_at           DATE NOT NULL,
    fired_at           DATE,
    -- permanent — постоянный, intern — стажёр, fired — уволенный
    employee_type      VARCHAR(20) NOT NULL DEFAULT 'permanent',
    photo_key          TEXT,
    user_id            INT REFERENCES users(id) ON DELETE SET NULL,
    branch_id          INT REFERENCES branches(id) ON DELETE SET NULL,
    created_by         INT REFERENCES users(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at         TIMESTAMPTZ,
    CONSTRAINT employees_type_chk CHECK (employee_type IN ('permanent', 'intern', 'fired')),
    CONSTRAINT employees_iin_chk CHECK (iin ~ '^[0-9]{12}$')
);

CREATE UNIQUE INDEX IF NOT EXISTS employees_iin_uniq ON employees(iin) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS employees_type_idx ON employees(employee_type) WHERE deleted_at IS NULL;

-- 2) Шаблоны кадровых документов (Word), загружают админ и отдел кадров.
CREATE TABLE IF NOT EXISTS hr_document_templates (
    id            SERIAL PRIMARY KEY,
    name          TEXT NOT NULL,
    -- labor_contract | nda | non_compete | liability | job_description | other
    kind          VARCHAR(30) NOT NULL DEFAULT 'other',
    file_key      TEXT NOT NULL,
    original_name TEXT,
    uploaded_by   INT REFERENCES users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);

-- 3) Кадровые документы — строки общей таблицы documents (scope = 'hr'):
--    подписание кодом работает тем же механизмом, что и у клиентов.
ALTER TABLE documents
    ADD COLUMN IF NOT EXISTS employee_id    INT REFERENCES employees(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS hr_template_id INT REFERENCES hr_document_templates(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS sent_at        TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS sent_via       VARCHAR(10),
    ADD COLUMN IF NOT EXISTS sent_to        TEXT,
    ADD COLUMN IF NOT EXISTS revoked_at     TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS documents_employee_idx ON documents(employee_id) WHERE employee_id IS NOT NULL;

-- Статус «Отозван» (revoked) для кадровых документов.
ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_status_chk;
ALTER TABLE documents ADD CONSTRAINT documents_status_chk CHECK (
    status IN ('draft', 'under_review', 'approved', 'returned', 'signed', 'sent_for_signature', 'revoked')
);
