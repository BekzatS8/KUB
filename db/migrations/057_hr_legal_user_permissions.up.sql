-- Добавить HR и юристу права на управление пользователями (через approval flow).
-- users.create / update / delete / block — действие не применяется напрямую:
-- handler маршрутизирует HR/legal на создание заявки (user_approval_requests).
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'department'
FROM roles r
JOIN permissions p ON p.code IN (
    'users.create', 'users.update', 'users.delete', 'users.block',
    'approvals.create', 'feed.view'
)
WHERE r.code IN ('hr', 'legal')
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
