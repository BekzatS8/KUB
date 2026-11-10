-- 099_wazzup_channel_branches.up.sql
-- Канал мессенджера (в первую очередь Instagram) обслуживают несколько
-- филиалов: лиды из Instagram kubvisa видят KUB Шымкент и KUB Алматы, из
-- visario.kz — филиалы VISARIO (замечания заказчика 10.11.2026).
-- Основной филиал канала — wazzup_channels.branch_id (в него встаёт новый
-- лид), дополнительные — здесь. Лид канала виден всем его филиалам;
-- видимость считается через чат лида, поэтому распространяется и на уже
-- накопленные лиды. Идемпотентно.
CREATE TABLE IF NOT EXISTS wazzup_channel_branches (
    channel_id BIGINT NOT NULL REFERENCES wazzup_channels(id) ON DELETE CASCADE,
    branch_id  INT    NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (channel_id, branch_id)
);

CREATE INDEX IF NOT EXISTS wazzup_channel_branches_branch_idx ON wazzup_channel_branches(branch_id);
CREATE INDEX IF NOT EXISTS chats_external_channel_idx ON chats(external_channel_id) WHERE external_channel_id IS NOT NULL;
