BEGIN;

DROP INDEX IF EXISTS clients_branch_id_idx;

ALTER TABLE clients
    DROP COLUMN IF EXISTS branch_id;

COMMIT;
