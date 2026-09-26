-- Номера Wazzup, удалённые вручную на странице «Каналы мессенджера».
-- Wazzup отвечает на удаление успехом, но заблокированный Meta номер
-- продолжает отдавать в списке каналов, и синхронизация возвращала его в CRM.
-- Такие номера пропускаются при синхронизации, пока снова не заработают.
CREATE TABLE IF NOT EXISTS wazzup_hidden_channels (
    integration_id      INT         NOT NULL REFERENCES wazzup_integrations(id) ON DELETE CASCADE,
    external_channel_id TEXT        NOT NULL,
    hidden_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (integration_id, external_channel_id)
);
