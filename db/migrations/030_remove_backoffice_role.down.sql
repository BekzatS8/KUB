BEGIN;

INSERT INTO roles (id, name, description)
SELECT 15, 'backoffice_admin_staff', 'Административный персонал (legacy rollback role)'
WHERE NOT EXISTS (SELECT 1 FROM roles WHERE id = 15);

UPDATE users
SET role_id = 15
WHERE role_id = 20
  AND EXISTS (SELECT 1 FROM roles WHERE id = 15);

COMMIT;
