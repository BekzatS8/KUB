-- 002_extend_clients.up.sql
-- Расширение таблицы clients под анкету физ. лиц

ALTER TABLE clients
    ADD COLUMN IF NOT EXISTS last_name            VARCHAR(255),
    ADD COLUMN IF NOT EXISTS first_name           VARCHAR(255),
    ADD COLUMN IF NOT EXISTS middle_name          VARCHAR(255),
    ADD COLUMN IF NOT EXISTS iin                  VARCHAR(20),
    ADD COLUMN IF NOT EXISTS id_number            VARCHAR(50),
    ADD COLUMN IF NOT EXISTS passport_series      VARCHAR(20),
    ADD COLUMN IF NOT EXISTS passport_number      VARCHAR(50),
    ADD COLUMN IF NOT EXISTS phone                VARCHAR(50),
    ADD COLUMN IF NOT EXISTS email                VARCHAR(255),
    ADD COLUMN IF NOT EXISTS registration_address TEXT,
    ADD COLUMN IF NOT EXISTS actual_address       TEXT;

CREATE INDEX IF NOT EXISTS idx_clients_iin   ON clients (iin);
CREATE INDEX IF NOT EXISTS idx_clients_phone ON clients (phone);
