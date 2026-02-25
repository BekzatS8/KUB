BEGIN;

DROP INDEX IF EXISTS idx_clients_client_type;

ALTER TABLE IF EXISTS clients
    DROP CONSTRAINT IF EXISTS clients_client_type_check;

ALTER TABLE IF EXISTS clients
    DROP COLUMN IF EXISTS client_type;

COMMIT;
