BEGIN;

-- =============================================================================
-- Migration 036: RBAC matrix corrections per TZ.
-- Idempotent: all inserts use ON CONFLICT DO UPDATE; deletes are safe re-runs.
-- Does NOT touch: users table, operations role (already gone), destructive ops.
-- =============================================================================

-- 1. Remove deals.* from visa and partner
--    visa/partner handle leads+clients+documents; sales deals are ОП only.
DELETE FROM role_permissions
WHERE role_id IN (SELECT id FROM roles WHERE code IN ('visa', 'partner'))
  AND permission_id IN (SELECT id FROM permissions WHERE code LIKE 'deals.%');

-- 2. Add telephony.view to visa, partner (department scope)
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'department'
FROM roles r
JOIN permissions p ON p.code = 'telephony.view'
WHERE r.code IN ('visa', 'partner')
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- 3. Add telephony.view to quality_control (related_departments scope — matches their read access)
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'related_departments'
FROM roles r
JOIN permissions p ON p.code = 'telephony.view'
WHERE r.code = 'quality_control'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- 4. Add telephony.view + users.view to hr (department scope)
--    HR needs telephony for work; users.view for employee/staff management.
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'department'
FROM roles r
JOIN permissions p ON p.code IN ('telephony.view', 'users.view')
WHERE r.code = 'hr'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- 5. Add telephony.view + clients.view + users.view to legal (department scope)
--    Legal needs to view clients for contract/legal context.
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'department'
FROM roles r
JOIN permissions p ON p.code IN ('telephony.view', 'clients.view', 'users.view')
WHERE r.code = 'legal'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- 6. Add documents.create/update/send to quality_control (department scope)
--    QC can create/update/send docs in own department only; cannot delete.
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'department'
FROM roles r
JOIN permissions p ON p.code IN ('documents.create', 'documents.update', 'documents.send')
WHERE r.code = 'quality_control'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- 7. Add users.view to management (related_departments scope — limited employee oversight)
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'related_departments'
FROM roles r
JOIN permissions p ON p.code = 'users.view'
WHERE r.code = 'management'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

COMMIT;
