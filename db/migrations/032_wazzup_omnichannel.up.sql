BEGIN;

CREATE TABLE IF NOT EXISTS wazzup_channels (
    id                  BIGSERIAL PRIMARY KEY,
    integration_id      INT NOT NULL REFERENCES wazzup_integrations(id) ON DELETE CASCADE,
    external_channel_id TEXT NOT NULL,
    transport           TEXT NOT NULL,
    name                TEXT,
    username            TEXT,
    phone               TEXT,
    status              TEXT,
    provider            TEXT NOT NULL DEFAULT 'wazzup',
    raw_payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (integration_id, external_channel_id)
);

CREATE INDEX IF NOT EXISTS wazzup_channels_transport_idx
    ON wazzup_channels(transport);

CREATE INDEX IF NOT EXISTS wazzup_channels_status_idx
    ON wazzup_channels(status);

ALTER TABLE chats
    ADD COLUMN IF NOT EXISTS external_provider TEXT,
    ADD COLUMN IF NOT EXISTS external_transport TEXT,
    ADD COLUMN IF NOT EXISTS external_chat_id TEXT,
    ADD COLUMN IF NOT EXISTS external_channel_id TEXT,
    ADD COLUMN IF NOT EXISTS external_display_name TEXT,
    ADD COLUMN IF NOT EXISTS external_username TEXT,
    ADD COLUMN IF NOT EXISTS external_phone TEXT,
    ADD COLUMN IF NOT EXISTS external_raw_payload JSONB,
    ADD COLUMN IF NOT EXISTS external_last_message_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS external_last_inbound_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS external_last_outbound_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS client_ref_id INT REFERENCES clients(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS lead_ref_id INT REFERENCES leads(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX IF NOT EXISTS chats_external_wazzup_uq
    ON chats(external_provider, external_transport, external_chat_id, COALESCE(external_channel_id, ''))
    WHERE external_provider IS NOT NULL
      AND external_chat_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS chats_external_provider_transport_idx
    ON chats(external_provider, external_transport);

CREATE INDEX IF NOT EXISTS chats_external_last_message_idx
    ON chats(external_last_message_at DESC NULLS LAST);

CREATE INDEX IF NOT EXISTS chats_client_ref_idx
    ON chats(client_ref_id);

CREATE INDEX IF NOT EXISTS chats_lead_ref_idx
    ON chats(lead_ref_id);

ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS external_provider TEXT,
    ADD COLUMN IF NOT EXISTS external_transport TEXT,
    ADD COLUMN IF NOT EXISTS external_message_id TEXT,
    ADD COLUMN IF NOT EXISTS external_channel_id TEXT,
    ADD COLUMN IF NOT EXISTS external_direction TEXT,
    ADD COLUMN IF NOT EXISTS external_status TEXT,
    ADD COLUMN IF NOT EXISTS external_raw_payload JSONB;

CREATE UNIQUE INDEX IF NOT EXISTS messages_external_wazzup_uq
    ON messages(external_provider, external_message_id)
    WHERE external_provider IS NOT NULL
      AND external_message_id IS NOT NULL
      AND external_message_id <> '';

CREATE INDEX IF NOT EXISTS messages_external_chat_idx
    ON messages(chat_id, external_direction, created_at DESC);

COMMIT;
