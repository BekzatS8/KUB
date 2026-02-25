BEGIN;

ALTER TABLE IF EXISTS clients
    ADD COLUMN IF NOT EXISTS client_type TEXT NOT NULL DEFAULT 'individual';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'clients_client_type_check'
    ) THEN
        ALTER TABLE clients
            ADD CONSTRAINT clients_client_type_check
                CHECK (client_type IN ('individual', 'legal'));
    END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_clients_client_type ON clients(client_type);

COMMIT;
