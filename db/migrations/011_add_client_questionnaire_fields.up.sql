BEGIN;

ALTER TABLE IF EXISTS clients
    ADD COLUMN IF NOT EXISTS country              TEXT,
    ADD COLUMN IF NOT EXISTS trip_purpose         TEXT,
    ADD COLUMN IF NOT EXISTS birth_date           DATE,
    ADD COLUMN IF NOT EXISTS birth_place          TEXT,
    ADD COLUMN IF NOT EXISTS citizenship          TEXT,
    ADD COLUMN IF NOT EXISTS sex                  VARCHAR(20),
    ADD COLUMN IF NOT EXISTS marital_status       VARCHAR(50),
    ADD COLUMN IF NOT EXISTS passport_issue_date  DATE,
    ADD COLUMN IF NOT EXISTS passport_expire_date DATE;

CREATE INDEX IF NOT EXISTS clients_country_idx
    ON clients (country);

CREATE TABLE IF NOT EXISTS client_files (
    id          BIGSERIAL PRIMARY KEY,
    client_id   INT NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    category    VARCHAR(50) NOT NULL,
    file_path   TEXT NOT NULL,
    mime        TEXT,
    size_bytes  BIGINT,
    uploaded_by INT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_primary  BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE INDEX IF NOT EXISTS client_files_client_id_idx
    ON client_files (client_id);

CREATE INDEX IF NOT EXISTS client_files_client_id_category_idx
    ON client_files (client_id, category);

CREATE UNIQUE INDEX IF NOT EXISTS client_files_primary_unique_idx
    ON client_files (client_id, category)
    WHERE is_primary = TRUE;

COMMIT;
