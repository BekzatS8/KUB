BEGIN;

DROP INDEX IF EXISTS users_branch_id_idx;

ALTER TABLE users
    DROP COLUMN IF EXISTS branch_id,
    DROP COLUMN IF EXISTS first_name,
    DROP COLUMN IF EXISTS last_name,
    DROP COLUMN IF EXISTS middle_name,
    DROP COLUMN IF EXISTS position,
    DROP COLUMN IF EXISTS is_active;

DROP TABLE IF EXISTS branches;

COMMIT;
