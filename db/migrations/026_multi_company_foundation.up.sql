BEGIN;

CREATE TABLE IF NOT EXISTS companies (
    id           SERIAL PRIMARY KEY,
    name         VARCHAR(255) NOT NULL UNIQUE,
    legal_name   VARCHAR(255),
    bin_iin      VARCHAR(32),
    company_type VARCHAR(100) NOT NULL,
    phone        VARCHAR(50),
    email        VARCHAR(255),
    address      TEXT,
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_companies (
    id         SERIAL PRIMARY KEY,
    user_id    INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    company_id INT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, company_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS user_companies_primary_uq
    ON user_companies(user_id)
    WHERE is_primary = TRUE AND is_active = TRUE;
CREATE INDEX IF NOT EXISTS user_companies_user_idx ON user_companies(user_id);
CREATE INDEX IF NOT EXISTS user_companies_company_idx ON user_companies(company_id);

INSERT INTO companies (name, legal_name, bin_iin, company_type, is_active)
VALUES
    ('KUB Visa Center', 'KUB Visa Center', NULL, 'visa_center', TRUE),
    ('VISARIO', 'VISARIO', NULL, 'visa_center', TRUE),
    ('Visa Flex', 'Visa Flex', NULL, 'visa_center', TRUE),
    ('KVMC', 'KVMC', NULL, 'visa_center', TRUE),
    ('KUB Digital Academy', 'KUB Digital Academy', NULL, 'academy', TRUE)
ON CONFLICT (name) DO UPDATE
SET
    legal_name = EXCLUDED.legal_name,
    company_type = EXCLUDED.company_type,
    is_active = EXCLUDED.is_active,
    updated_at = NOW();

-- Backfill by exact company_name -> companies.name match.
INSERT INTO user_companies (user_id, company_id, is_primary, is_active)
SELECT u.id, c.id, TRUE, TRUE
FROM users u
JOIN companies c ON c.name = u.company_name
ON CONFLICT (user_id, company_id) DO NOTHING;

COMMIT;
