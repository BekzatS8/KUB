BEGIN;

-- =============================================================================
-- Step 1: Add `code` column to roles (idempotent)
-- =============================================================================
ALTER TABLE roles
    ADD COLUMN IF NOT EXISTS code VARCHAR(64);

-- =============================================================================
-- Step 2: Assign canonical codes to the ACTUAL production role IDs.
--
-- Production schema (queried 2026-06-11):
--   id=1  name='lawyer'     → code='legal'
--   id=2  name='hr'         → code='hr'
--   id=3  name='bdm'        → code='partner'
--   id=10 name='sales'      → code='sales'
--   id=20 name='vds'        → code='visa'   (Визовый отдел — NOT operations)
--   id=30 name='audit'      → code='quality_control'
--   id=40 name='management' → code='management'
--   id=50 name='admin'      → code='admin'
--
-- Using ELSE code (no-op) for any unknown role IDs so we never auto-derive
-- a code that could later conflict with a subsequent INSERT.
-- Safe to run repeatedly: same IDs → same codes, no unique constraint violation.
-- =============================================================================
UPDATE roles
SET code = CASE id
    WHEN  1 THEN 'legal'
    WHEN  2 THEN 'hr'
    WHEN  3 THEN 'partner'
    WHEN 10 THEN 'sales'
    WHEN 20 THEN 'visa'
    WHEN 30 THEN 'quality_control'
    WHEN 40 THEN 'management'
    WHEN 50 THEN 'admin'
    ELSE code
END
WHERE id IN (1, 2, 3, 10, 20, 30, 40, 50);

-- =============================================================================
-- Step 3: Unique index on code (idempotent: IF NOT EXISTS)
-- =============================================================================
CREATE UNIQUE INDEX IF NOT EXISTS roles_code_uq
    ON roles(code)
    WHERE code IS NOT NULL;

-- =============================================================================
-- Step 4: Departments table (idempotent: IF NOT EXISTS + ON CONFLICT)
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
-- Step 5: department_id on users (idempotent: IF NOT EXISTS)
-- =============================================================================
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS department_id INT REFERENCES departments(id);

CREATE INDEX IF NOT EXISTS users_department_id_idx ON users(department_id);

-- =============================================================================
-- Step 6: Permissions tables (idempotent: IF NOT EXISTS + ON CONFLICT DO NOTHING)
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
-- Step 7: Seed role_permissions (idempotent: ON CONFLICT DO UPDATE)
-- All lookups by role code, so production role IDs are irrelevant here.
-- =============================================================================

-- admin: all permissions
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'all'
FROM roles r CROSS JOIN permissions p
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- management
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
    'reports.view', 'chat.view', 'messenger.view', 'telephony.view', 'funnels.view'
)
WHERE r.code = 'management'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- quality_control (was 'audit' by name, code='quality_control')
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'related_departments'
FROM roles r
JOIN permissions p ON p.code IN (
    'feed.view', 'leads.view', 'deals.view', 'clients.view',
    'documents.view', 'documents.download',
    'tasks.view', 'reports.view', 'chat.view', 'messenger.view', 'funnels.view'
)
WHERE r.code = 'quality_control'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- sales, visa (id=20/vds), partner (id=3/bdm)
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
WHERE r.code IN ('sales', 'visa', 'partner')
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- hr (id=2), legal (id=1/lawyer)
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'department'
FROM roles r
JOIN permissions p ON p.code IN (
    'feed.view',
    'documents.view', 'documents.create', 'documents.update', 'documents.download',
    'tasks.view', 'tasks.create', 'tasks.update',
    'chat.view', 'approvals.create'
)
WHERE r.code IN ('hr', 'legal')
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- =============================================================================
-- Step 8: Funnels table (idempotent: IF NOT EXISTS)
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
-- Step 9: funnel_id on leads and deals (idempotent: IF NOT EXISTS)
-- =============================================================================
ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS funnel_id INT REFERENCES funnels(id);

ALTER TABLE deals
    ADD COLUMN IF NOT EXISTS funnel_id INT REFERENCES funnels(id);

CREATE INDEX IF NOT EXISTS leads_funnel_id_idx ON leads(funnel_id);
CREATE INDEX IF NOT EXISTS deals_funnel_id_idx ON deals(funnel_id);

-- =============================================================================
-- Step 10: Default funnels (idempotent: ON CONFLICT DO NOTHING)
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
