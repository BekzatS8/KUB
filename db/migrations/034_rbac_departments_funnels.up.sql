BEGIN;

ALTER TABLE roles
    ADD COLUMN IF NOT EXISTS code VARCHAR(64);

UPDATE roles
SET code = CASE id
    WHEN 10 THEN 'sales'
    WHEN 30 THEN 'quality_control'
    WHEN 40 THEN 'management'
    WHEN 50 THEN 'admin'
    ELSE COALESCE(NULLIF(code, ''), LOWER(REGEXP_REPLACE(name, '[^a-zA-Z0-9]+', '_', 'g')))
END
WHERE code IS NULL OR code = '' OR id IN (10, 30, 40, 50);

-- role_id=20 was legacy operations. Keep it unused in the new RBAC foundation;
-- migrate any existing users explicitly before production rollout.

CREATE UNIQUE INDEX IF NOT EXISTS roles_code_uq
    ON roles(code)
    WHERE code IS NOT NULL;

INSERT INTO roles (id, name, code, description) VALUES
    (50, 'admin', 'admin', 'Администратор'),
    (10, 'sales', 'sales', 'Отдел продаж ОП'),
    (60, 'visa', 'visa', 'Визовый отдел ВО'),
    (70, 'partner', 'partner', 'Партнерский отдел ПО'),
    (80, 'hr', 'hr', 'Отдел кадров ОК'),
    (90, 'legal', 'legal', 'Юридический отдел ЮО'),
    (30, 'quality_control', 'quality_control', 'Отдел контроля качества ОКК'),
    (40, 'management', 'management', 'Руководство')
ON CONFLICT (id) DO UPDATE
SET code = EXCLUDED.code,
    description = COALESCE(NULLIF(roles.description, ''), EXCLUDED.description);

CREATE TABLE IF NOT EXISTS departments (
    id         SERIAL PRIMARY KEY,
    name       VARCHAR(255) NOT NULL,
    code       VARCHAR(64)  NOT NULL UNIQUE,
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO departments (name, code, is_active) VALUES
    ('Отдел продаж ОП', 'sales', TRUE),
    ('Визовый отдел ВО', 'visa', TRUE),
    ('Партнерский отдел ПО', 'partner', TRUE),
    ('Отдел кадров ОК', 'hr', TRUE),
    ('Юридический отдел ЮО', 'legal', TRUE),
    ('Отдел контроля качества ОКК', 'quality_control', TRUE),
    ('Руководство', 'management', TRUE),
    ('Администрация', 'admin', TRUE)
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    is_active = EXCLUDED.is_active,
    updated_at = NOW();

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS department_id INT REFERENCES departments(id);

CREATE INDEX IF NOT EXISTS users_department_id_idx ON users(department_id);

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
    ('leads.view'), ('leads.create'), ('leads.update'), ('leads.delete'), ('leads.transfer_manager'), ('leads.move_between_funnels'),
    ('deals.view'), ('deals.create'), ('deals.update'), ('deals.delete'),
    ('clients.view'), ('clients.create'), ('clients.update'), ('clients.delete'), ('clients.export'),
    ('documents.view'), ('documents.create'), ('documents.update'), ('documents.delete'), ('documents.send'), ('documents.download'),
    ('tasks.view'), ('tasks.create'), ('tasks.update'), ('tasks.delete'),
    ('users.view'), ('users.create'), ('users.update'), ('users.delete'), ('users.block'), ('users.move_department'), ('users.move_branch'),
    ('branches.view'), ('branches.create'), ('branches.update'), ('branches.delete'),
    ('departments.view'), ('departments.create'), ('departments.update'), ('departments.delete'),
    ('funnels.view'), ('funnels.create'), ('funnels.update'), ('funnels.delete'), ('funnels.reorder'),
    ('reports.view'),
    ('chat.view'), ('chat.delete'),
    ('messenger.view'),
    ('telephony.view'),
    ('approvals.view'), ('approvals.create'), ('approvals.approve'), ('approvals.reject')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'all'
FROM roles r
CROSS JOIN permissions p
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'related_departments'
FROM roles r
JOIN permissions p ON p.code IN (
    'feed.view', 'leads.view', 'leads.create', 'leads.update', 'leads.transfer_manager', 'leads.move_between_funnels',
    'deals.view', 'deals.create', 'deals.update',
    'clients.view', 'clients.create', 'clients.update',
    'documents.view', 'documents.create', 'documents.update', 'documents.send', 'documents.download',
    'tasks.view', 'tasks.create', 'tasks.update',
    'reports.view', 'chat.view', 'messenger.view', 'telephony.view', 'funnels.view'
)
WHERE r.code = 'management'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'related_departments'
FROM roles r
JOIN permissions p ON p.code IN (
    'feed.view', 'leads.view', 'deals.view', 'clients.view', 'documents.view', 'documents.download',
    'tasks.view', 'reports.view', 'chat.view', 'messenger.view', 'funnels.view'
)
WHERE r.code = 'quality_control'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'department'
FROM roles r
JOIN permissions p ON p.code IN (
    'feed.view', 'leads.view', 'leads.create', 'leads.update',
    'deals.view', 'deals.create', 'deals.update',
    'clients.view', 'clients.create', 'clients.update',
    'documents.view', 'documents.create', 'documents.update', 'documents.send', 'documents.download',
    'tasks.view', 'tasks.create', 'tasks.update',
    'chat.view', 'messenger.view', 'telephony.view', 'funnels.view', 'approvals.create'
)
WHERE r.code IN ('sales', 'visa', 'partner')
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'department'
FROM roles r
JOIN permissions p ON p.code IN (
    'feed.view', 'documents.view', 'documents.create', 'documents.update', 'documents.download',
    'tasks.view', 'tasks.create', 'tasks.update', 'chat.view', 'approvals.create'
)
WHERE r.code IN ('hr', 'legal')
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

DELETE FROM role_permissions rp
USING roles r
WHERE rp.role_id = r.id
  AND (r.id = 20 OR r.code = 'operations');

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
CREATE INDEX IF NOT EXISTS funnels_branch_id_idx ON funnels(branch_id);
CREATE INDEX IF NOT EXISTS funnels_active_idx ON funnels(is_active);

ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS funnel_id INT REFERENCES funnels(id);

ALTER TABLE deals
    ADD COLUMN IF NOT EXISTS funnel_id INT REFERENCES funnels(id);

CREATE INDEX IF NOT EXISTS leads_funnel_id_idx ON leads(funnel_id);
CREATE INDEX IF NOT EXISTS deals_funnel_id_idx ON deals(funnel_id);

INSERT INTO funnels (name, code, department_id, is_active, sort_order)
SELECT 'Продажи', 'sales_default', d.id, TRUE, 10
FROM departments d
WHERE d.code = 'sales'
ON CONFLICT (code) DO NOTHING;

INSERT INTO funnels (name, code, department_id, is_active, sort_order)
SELECT 'Визы', 'visa_default', d.id, TRUE, 20
FROM departments d
WHERE d.code = 'visa'
ON CONFLICT (code) DO NOTHING;

INSERT INTO funnels (name, code, department_id, is_active, sort_order)
SELECT 'Партнеры', 'partner_default', d.id, TRUE, 30
FROM departments d
WHERE d.code = 'partner'
ON CONFLICT (code) DO NOTHING;

COMMIT;
