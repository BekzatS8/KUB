-- 096_calendar_events.up.sql
-- ТЗ «доработки CRM КУБ», п.6: раздел «Календарь» — даты и время встреч с
-- клиентами в виде календаря. Заполняют обычно операторы и МОП.
-- Идемпотентно (миграции прогоняются при каждом деплое).

CREATE TABLE IF NOT EXISTS calendar_events (
    id          BIGSERIAL PRIMARY KEY,
    title       TEXT NOT NULL,
    description TEXT,
    starts_at   TIMESTAMPTZ NOT NULL,
    ends_at     TIMESTAMPTZ,
    -- встреча по лиду/клиенту (необязательно)
    lead_id     INT REFERENCES leads(id) ON DELETE SET NULL,
    client_id   INT REFERENCES clients(id) ON DELETE SET NULL,
    -- чья встреча (ответственный сотрудник) и кто её назначил
    owner_id    INT REFERENCES users(id) ON DELETE SET NULL,
    created_by  INT REFERENCES users(id) ON DELETE SET NULL,
    branch_id   INT REFERENCES branches(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS calendar_events_starts_idx ON calendar_events(starts_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS calendar_events_owner_idx ON calendar_events(owner_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS calendar_events_lead_idx ON calendar_events(lead_id) WHERE lead_id IS NOT NULL;
