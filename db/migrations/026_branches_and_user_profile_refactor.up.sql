BEGIN;

CREATE TABLE IF NOT EXISTS branches (
    id         SERIAL PRIMARY KEY,
    name       VARCHAR(255) NOT NULL,
    code       VARCHAR(64)  NOT NULL UNIQUE,
    address    TEXT,
    phone      VARCHAR(50),
    email      VARCHAR(255),
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO branches (name, code, is_active)
VALUES
    ('Branch 1', 'BRANCH_1', TRUE),
    ('Branch 2', 'BRANCH_2', TRUE),
    ('Branch 3', 'BRANCH_3', TRUE),
    ('Branch 4', 'BRANCH_4', TRUE),
    ('Branch 5', 'BRANCH_5', TRUE)
ON CONFLICT (code) DO NOTHING;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS branch_id INT REFERENCES branches(id),
    ADD COLUMN IF NOT EXISTS first_name VARCHAR(255),
    ADD COLUMN IF NOT EXISTS last_name VARCHAR(255),
    ADD COLUMN IF NOT EXISTS middle_name VARCHAR(255),
    ADD COLUMN IF NOT EXISTS position VARCHAR(255),
    ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE;

CREATE INDEX IF NOT EXISTS users_branch_id_idx ON users(branch_id);

-- Backfill note:
-- В текущей схеме безопасно вывести branch_id из legacy-полей users(company_name/bin_iin) нельзя,
-- поэтому оставляем users.branch_id = NULL для существующих записей.

COMMIT;
