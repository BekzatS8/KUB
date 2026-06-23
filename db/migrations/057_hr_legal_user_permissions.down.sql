DELETE FROM role_permissions
WHERE role_id IN (SELECT id FROM roles WHERE code IN ('hr', 'legal'))
  AND permission_id IN (SELECT id FROM permissions WHERE code IN (
      'users.create', 'users.update', 'users.delete', 'users.block'
  ));
