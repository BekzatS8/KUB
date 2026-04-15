BEGIN;

CREATE TABLE IF NOT EXISTS company_integrations (
    id                  BIGSERIAL PRIMARY KEY,
    company_id          INT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    integration_type    VARCHAR(50) NOT NULL,
    provider            VARCHAR(100),
    title               VARCHAR(255) NOT NULL,
    external_account_id VARCHAR(255),
    phone               VARCHAR(64),
    username            VARCHAR(255),
    meta_json           JSONB,
    is_active           BOOLEAN NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT company_integrations_type_chk CHECK (
        integration_type IN ('whatsapp', 'telegram', 'instagram', 'tiktok', 'ip_telephony', 'binotel')
    )
);

CREATE INDEX IF NOT EXISTS company_integrations_company_idx ON company_integrations(company_id);
CREATE INDEX IF NOT EXISTS company_integrations_company_type_idx ON company_integrations(company_id, integration_type);

COMMIT;
