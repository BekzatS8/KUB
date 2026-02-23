BEGIN;

ALTER TABLE IF EXISTS leads
    ADD COLUMN IF NOT EXISTS phone VARCHAR(50),
    ADD COLUMN IF NOT EXISTS source VARCHAR(30);

CREATE INDEX IF NOT EXISTS leads_phone_idx
    ON leads(phone);

CREATE TABLE IF NOT EXISTS wazzup_integrations (
    id            SERIAL PRIMARY KEY,
    owner_user_id INT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    api_key_enc   TEXT NOT NULL,
    crm_key_hash  TEXT NOT NULL,
    webhook_token TEXT NOT NULL UNIQUE,
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,
    webhooks_uri  TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS wazzup_integrations_owner_user_idx
    ON wazzup_integrations(owner_user_id);


CREATE UNIQUE INDEX IF NOT EXISTS wazzup_integrations_owner_user_uq
    ON wazzup_integrations(owner_user_id);

CREATE INDEX IF NOT EXISTS wazzup_integrations_enabled_idx
    ON wazzup_integrations(enabled);

CREATE TABLE IF NOT EXISTS wazzup_dedup (
    id             BIGSERIAL PRIMARY KEY,
    integration_id INT NOT NULL REFERENCES wazzup_integrations(id) ON DELETE CASCADE,
    external_id    TEXT NOT NULL,
    received_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (integration_id, external_id)
);

COMMIT;
