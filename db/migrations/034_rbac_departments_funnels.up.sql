BEGIN;

-- =============================================================================
-- Step 1: Add `code` column to roles (idempotent)
-- =============================================================================
ALTER TABLE roles
    ADD COLUMN IF NOT EXISTS code VARCHAR(64);

-- =============================================================================
-- Step 2: Pre-cleanup — remove stale/conflicting codes from WRONG rows.
--
-- Canonical mapping:
--   id=10  → code='sales'
--   id=30  → code='quality_control'
--   id=40  → code='management'
--   id=50  → code='admin'
--   id=60  → code='visa'
--   id=70  → code='partner'
--   id=80  → code='hr'
--   id=90  → code='legal'
--
-- Any row carrying one of these codes at a DIFFERENT id gets code cleared.
-- Also clears code='operations' from any row — deleted legacy role.
--
-- Safe on clean DB (no non-null codes → no rows updated).
-- Safe on repeat run (correct codes on correct rows → no rows updated).
-- Must run BEFORE the unique index so no constraint fires during cleanup.
-- =============================================================================
UPDATE roles
SET code = NULL
WHERE (code = 'sales'           AND id != 10)
   OR (code = 'quality_control' AND id != 30)
   OR (code = 'management'      AND id != 40)
   OR (code = 'admin'           AND id != 50)
   OR (code = 'visa'            AND id != 60)
   OR (code = 'partner'         AND id != 70)
   OR (code = 'hr'              AND id != 80)
   OR (code = 'legal'           AND id != 90)
   OR  code = 'operations';

-- =============================================================================
-- Step 3: Unique index on code (idempotent: IF NOT EXISTS)
-- =============================================================================
CREATE UNIQUE INDEX IF NOT EXISTS roles_code_uq
    ON roles(code)
    WHERE code IS NOT NULL;

-- =============================================================================
-- Step 4: Seed / upsert canonical roles (idempotent: ON CONFLICT (id) DO UPDATE)
--
-- Active roles:
--   id=10  code='sales'
--   id=30  code='quality_control'
--   id=40  code='management'
--   id=50  code='admin'
--   id=60  code='visa'
--   id=70  code='partner'
--   id=80  code='hr'
--   id=90  code='legal'
--
-- role_id=20 / code='operations' is NOT seeded here — it is a deleted legacy role.
-- =============================================================================
INSERT INTO roles (id, name, description, code) VALUES
    (10, 'sales',           'Отдел продаж: лиды/сделки/документы',  'sales'),
    (30, 'audit',           'Контроль качества: наблюдатель',        'quality_control'),
    (40, 'management',      'Руководство: расширенный доступ',       'management'),
    (50, 'admin',           'Администратор: управление системой',    'admin'),
    (60, 'visa',            'Визовый отдел: визовые услуги',         'visa'),
    (70, 'partner',         'Партнёрский отдел',                     'partner'),
    (80, 'hr',              'Отдел кадров',                          'hr'),
    (90, 'legal',           'Юридический отдел',                     'legal')
ON CONFLICT (id) DO UPDATE
    SET code = EXCLUDED.code;

-- =============================================================================
-- Step 5: Safety — migrate any users still on role_id=20 to visa (id=60).
-- Idempotent: if no users have role_id=20 this is a no-op.
-- Protects FK constraint before deleting the role row.
-- =============================================================================
UPDATE users
SET role_id = 60
WHERE role_id = 20;

-- =============================================================================
-- Step 6: Remove role_permissions for role_id=20 (legacy operations).
-- Idempotent: no-op if already absent.
-- =============================================================================
DELETE FROM role_permissions WHERE role_id = 20;

-- =============================================================================
-- Step 7: Delete legacy role id=20 / code='operations'.
-- FK on users is safe because Step 5 reassigned all role_id=20 users.
-- =============================================================================
DELETE FROM roles
WHERE id = 20
   OR code = 'operations';

-- =============================================================================
-- Step 8: Advance SERIAL sequence past our highest explicit id (90).
-- =============================================================================
SELECT setval('roles_id_seq', GREATEST(90, COALESCE((SELECT MAX(id) FROM roles), 90)));

-- =============================================================================
-- Step 9: Departments table (idempotent: IF NOT EXISTS + ON CONFLICT)
-- =============================================================================
CREATE TABLE IF NOT EXISTS departments (
    id         SERIAL PRIMARY KEY,
    name       VARCHAR(255) NOT NULL,
    code       VARCHAR(64)  NOT NULL UNIQUE,
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO departments (name, code, is_active) VALUES
    ('Отдел продаж ОП',             'sales',           TRUE),
    ('Визовый отдел ВО',            'visa',            TRUE),
    ('Партнерский отдел ПО',        'partner',         TRUE),
    ('Отдел кадров ОК',             'hr',              TRUE),
    ('Юридический отдел ЮО',        'legal',           TRUE),
    ('Отдел контроля качества ОКК', 'quality_control', TRUE),
    ('Руководство',                 'management',      TRUE),
    ('Администрация',               'admin',           TRUE)
ON CONFLICT (code) DO UPDATE
    SET name      = EXCLUDED.name,
        is_active = EXCLUDED.is_active,
        updated_at = NOW();

-- =============================================================================
-- Step 10: department_id on users (idempotent: IF NOT EXISTS, nullable)
-- =============================================================================
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS department_id INT REFERENCES departments(id);

CREATE INDEX IF NOT EXISTS users_department_id_idx ON users(department_id);

-- =============================================================================
-- Step 11: Permissions tables (idempotent: IF NOT EXISTS + ON CONFLICT DO NOTHING)
-- =============================================================================
CREATE TABLE IF NOT EXISTS permissions (
    id          SERIAL PRIMARY KEY,
    code        VARCHAR(128) NOT NULL UNIQUE,
    description TEXT
);

CREATE TABLE IF NOT EXISTS role_permissions (
    role_id       INT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id INT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    scope         VARCHAR(64) NOT NULL,
    PRIMARY KEY (role_id, permission_id)
);

INSERT INTO permissions (code) VALUES
    ('feed.view'),
    ('leads.view'), ('leads.create'), ('leads.update'), ('leads.delete'),
    ('leads.transfer_manager'), ('leads.move_between_funnels'),
    ('deals.view'), ('deals.create'), ('deals.update'), ('deals.delete'),
    ('clients.view'), ('clients.create'), ('clients.update'), ('clients.delete'), ('clients.export'),
    ('documents.view'), ('documents.create'), ('documents.update'), ('documents.delete'),
    ('documents.send'), ('documents.download'),
    ('tasks.view'), ('tasks.create'), ('tasks.update'), ('tasks.delete'),
    ('users.view'), ('users.create'), ('users.update'), ('users.delete'),
    ('users.block'), ('users.move_department'), ('users.move_branch'),
    ('branches.view'), ('branches.create'), ('branches.update'), ('branches.delete'),
    ('departments.view'), ('departments.create'), ('departments.update'), ('departments.delete'),
    ('funnels.view'), ('funnels.create'), ('funnels.update'), ('funnels.delete'), ('funnels.reorder'),
    ('reports.view'),
    ('chat.view'), ('chat.delete'),
    ('messenger.view'),
    ('telephony.view'),
    ('approvals.view'), ('approvals.create'), ('approvals.approve'), ('approvals.reject')
ON CONFLICT (code) DO NOTHING;

-- =============================================================================
-- Step 12: Seed role_permissions for the 8 active roles only.
-- All lookups use r.code, not hardcoded role_id values.
-- No permissions seeded for role_id=20 or code='operations'.
-- =============================================================================

-- admin: full access to all permissions
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'all'
FROM roles r CROSS JOIN permissions p
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- management: broad access across related departments; users.view for limited employee oversight
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'related_departments'
FROM roles r
JOIN permissions p ON p.code IN (
    'feed.view',
    'leads.view', 'leads.create', 'leads.update', 'leads.transfer_manager', 'leads.move_between_funnels',
    'deals.view', 'deals.create', 'deals.update',
    'clients.view', 'clients.create', 'clients.update',
    'documents.view', 'documents.create', 'documents.update', 'documents.send', 'documents.download',
    'tasks.view', 'tasks.create', 'tasks.update',
    'reports.view', 'chat.view', 'messenger.view', 'telephony.view', 'funnels.view',
    'users.view'
)
WHERE r.code = 'management'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- quality_control: read-only for leads/deals/clients + limited document writes (department scope) + telephony
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'related_departments'
FROM roles r
JOIN permissions p ON p.code IN (
    'feed.view', 'leads.view', 'deals.view', 'clients.view',
    'documents.view', 'documents.download',
    'tasks.view', 'reports.view', 'chat.view', 'messenger.view', 'telephony.view', 'funnels.view'
)
WHERE r.code = 'quality_control'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- quality_control: own-department document writes only (auditors can create/update/send docs in their dept)
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'department'
FROM roles r
JOIN permissions p ON p.code IN (
    'documents.create', 'documents.update', 'documents.send'
)
WHERE r.code = 'quality_control'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- sales: department-scoped business access including deals
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'department'
FROM roles r
JOIN permissions p ON p.code IN (
    'feed.view',
    'leads.view', 'leads.create', 'leads.update',
    'deals.view', 'deals.create', 'deals.update',
    'clients.view', 'clients.create', 'clients.update',
    'documents.view', 'documents.create', 'documents.update', 'documents.send', 'documents.download',
    'tasks.view', 'tasks.create', 'tasks.update',
    'chat.view', 'messenger.view', 'telephony.view', 'funnels.view', 'approvals.create'
)
WHERE r.code = 'sales'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- visa, partner: department-scoped access; NO deals.* (they handle leads/clients/docs only)
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'department'
FROM roles r
JOIN permissions p ON p.code IN (
    'feed.view',
    'leads.view', 'leads.create', 'leads.update',
    'clients.view', 'clients.update',
    'documents.view', 'documents.create', 'documents.update', 'documents.send', 'documents.download',
    'tasks.view', 'tasks.create', 'tasks.update',
    'chat.view', 'messenger.view', 'telephony.view', 'funnels.view', 'approvals.create'
)
WHERE r.code IN ('visa', 'partner')
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- hr: employee/document management; no leads/deals/messenger
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'department'
FROM roles r
JOIN permissions p ON p.code IN (
    'feed.view', 'users.view',
    'documents.view', 'documents.create', 'documents.update', 'documents.download',
    'tasks.view', 'tasks.create', 'tasks.update',
    'chat.view', 'telephony.view', 'approvals.create'
)
WHERE r.code = 'hr'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- legal: clients+documents access; no leads/deals/messenger
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'department'
FROM roles r
JOIN permissions p ON p.code IN (
    'feed.view', 'clients.view', 'users.view',
    'documents.view', 'documents.create', 'documents.update', 'documents.download',
    'tasks.view', 'tasks.create', 'tasks.update',
    'chat.view', 'telephony.view', 'approvals.create'
)
WHERE r.code = 'legal'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- =============================================================================
-- Step 13: Funnels table (idempotent: IF NOT EXISTS)
-- =============================================================================
CREATE TABLE IF NOT EXISTS funnels (
    id            SERIAL PRIMARY KEY,
    name          VARCHAR(255) NOT NULL,
    code          VARCHAR(64) NOT NULL UNIQUE,
    department_id INT NOT NULL REFERENCES departments(id),
    branch_id     INT REFERENCES branches(id),
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order    INT NOT NULL DEFAULT 0,
    created_by    INT REFERENCES users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS funnels_department_id_idx ON funnels(department_id);
CREATE INDEX IF NOT EXISTS funnels_branch_id_idx     ON funnels(branch_id);
CREATE INDEX IF NOT EXISTS funnels_active_idx        ON funnels(is_active);

-- =============================================================================
-- Step 14: funnel_id on leads and deals (idempotent: IF NOT EXISTS)
-- =============================================================================
ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS funnel_id INT REFERENCES funnels(id);

ALTER TABLE deals
    ADD COLUMN IF NOT EXISTS funnel_id INT REFERENCES funnels(id);

CREATE INDEX IF NOT EXISTS leads_funnel_id_idx ON leads(funnel_id);
CREATE INDEX IF NOT EXISTS deals_funnel_id_idx ON deals(funnel_id);

-- =============================================================================
-- Step 15: Default funnels (idempotent: ON CONFLICT DO NOTHING)
-- =============================================================================
INSERT INTO funnels (name, code, department_id, is_active, sort_order)
SELECT 'Продажи', 'sales_default', d.id, TRUE, 10
FROM departments d WHERE d.code = 'sales'
ON CONFLICT (code) DO NOTHING;

INSERT INTO funnels (name, code, department_id, is_active, sort_order)
SELECT 'Визы', 'visa_default', d.id, TRUE, 20
FROM departments d WHERE d.code = 'visa'
ON CONFLICT (code) DO NOTHING;

INSERT INTO funnels (name, code, department_id, is_active, sort_order)
SELECT 'Партнеры', 'partner_default', d.id, TRUE, 30
FROM departments d WHERE d.code = 'partner'
ON CONFLICT (code) DO NOTHING;

COMMIT;
