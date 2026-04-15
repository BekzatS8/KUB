BEGIN;

UPDATE users
SET role_id = 20
WHERE role_id = 15;

DELETE FROM roles
WHERE id = 15;

COMMIT;
