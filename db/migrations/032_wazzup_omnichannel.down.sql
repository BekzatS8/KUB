BEGIN;

DROP INDEX IF EXISTS messages_external_chat_idx;
DROP INDEX IF EXISTS messages_external_wazzup_uq;

ALTER TABLE messages
    DROP COLUMN IF EXISTS external_raw_payload,
    DROP COLUMN IF EXISTS external_status,
    DROP COLUMN IF EXISTS external_direction,
    DROP COLUMN IF EXISTS external_channel_id,
    DROP COLUMN IF EXISTS external_message_id,
    DROP COLUMN IF EXISTS external_transport,
    DROP COLUMN IF EXISTS external_provider;

DROP INDEX IF EXISTS chats_lead_ref_idx;
DROP INDEX IF EXISTS chats_client_ref_idx;
DROP INDEX IF EXISTS chats_external_last_message_idx;
DROP INDEX IF EXISTS chats_external_provider_transport_idx;
DROP INDEX IF EXISTS chats_external_wazzup_uq;

ALTER TABLE chats
    DROP COLUMN IF EXISTS lead_ref_id,
    DROP COLUMN IF EXISTS client_ref_id,
    DROP COLUMN IF EXISTS external_last_outbound_at,
    DROP COLUMN IF EXISTS external_last_inbound_at,
    DROP COLUMN IF EXISTS external_last_message_at,
    DROP COLUMN IF EXISTS external_raw_payload,
    DROP COLUMN IF EXISTS external_phone,
    DROP COLUMN IF EXISTS external_username,
    DROP COLUMN IF EXISTS external_display_name,
    DROP COLUMN IF EXISTS external_channel_id,
    DROP COLUMN IF EXISTS external_chat_id,
    DROP COLUMN IF EXISTS external_transport,
    DROP COLUMN IF EXISTS external_provider;

DROP INDEX IF EXISTS wazzup_channels_status_idx;
DROP INDEX IF EXISTS wazzup_channels_transport_idx;
DROP TABLE IF EXISTS wazzup_channels;

COMMIT;
