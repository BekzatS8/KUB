-- 037_telephony_calls.up.sql
-- Idempotent: safe to run multiple times

CREATE TABLE IF NOT EXISTS telephony_calls (
    id               BIGSERIAL PRIMARY KEY,
    provider         TEXT        NOT NULL DEFAULT 'binotel',
    external_call_id TEXT        NULL,
    direction        TEXT        NOT NULL DEFAULT 'inbound',
    status           TEXT        NOT NULL DEFAULT 'unknown',
    phone            TEXT        NOT NULL DEFAULT '',
    normalized_phone TEXT        NULL,
    client_id        BIGINT      NULL REFERENCES clients(id)  ON DELETE SET NULL,
    lead_id          BIGINT      NULL REFERENCES leads(id)    ON DELETE SET NULL,
    manager_id       INT         NULL REFERENCES users(id)    ON DELETE SET NULL,
    branch_id        INT         NULL REFERENCES branches(id) ON DELETE SET NULL,
    started_at       TIMESTAMPTZ NULL,
    answered_at      TIMESTAMPTZ NULL,
    ended_at         TIMESTAMPTZ NULL,
    duration_seconds INT         NULL,
    recording_url    TEXT        NULL,
    raw_payload      JSONB       NOT NULL DEFAULT '{}',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Unique partial index for idempotency: one record per (provider, external_call_id)
CREATE UNIQUE INDEX IF NOT EXISTS idx_telephony_calls_provider_external
    ON telephony_calls (provider, external_call_id)
    WHERE external_call_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_telephony_calls_normalized_phone
    ON telephony_calls (normalized_phone)
    WHERE normalized_phone IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_telephony_calls_client_id
    ON telephony_calls (client_id)
    WHERE client_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_telephony_calls_lead_id
    ON telephony_calls (lead_id)
    WHERE lead_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_telephony_calls_manager_id
    ON telephony_calls (manager_id)
    WHERE manager_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_telephony_calls_branch_id
    ON telephony_calls (branch_id)
    WHERE branch_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_telephony_calls_started_at
    ON telephony_calls (started_at DESC NULLS LAST);

CREATE INDEX IF NOT EXISTS idx_telephony_calls_status
    ON telephony_calls (status);
