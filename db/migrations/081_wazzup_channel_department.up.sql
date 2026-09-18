-- 081_wazzup_channel_department.up.sql
-- Канал мессенджера можно привязать не только к филиалу, но и к ОТДЕЛУ
-- (обратная связь заказчика 18.09.2026): отдельный номер WhatsApp для отдела
-- контроля качества — жалобы, претензии. Такие обращения не должны попадать
-- в общий пул лидов, который разбирают менеджеры филиалов.
--
-- Модель:
--   wazzup_channels.department_id — куда направлять входящие с этого канала;
--   departments.is_private        — переписка отдела закрыта от других отделов.
--
-- This migration is re-applied on every deploy, so every statement is idempotent.

ALTER TABLE wazzup_channels
    ADD COLUMN IF NOT EXISTS department_id INT REFERENCES departments(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS wazzup_channels_department_id_idx
    ON wazzup_channels(department_id);

ALTER TABLE departments
    ADD COLUMN IF NOT EXISTS is_private BOOLEAN NOT NULL DEFAULT FALSE;

-- Отдел контроля качества разбирает жалобы и претензии: его лиды видят только
-- сам отдел, руководство и админ. Остальные отделы остаются открытыми, поэтому
-- видимость уже существующих лидов не меняется.
UPDATE departments SET is_private = TRUE WHERE code = 'quality_control' AND is_private = FALSE;

CREATE INDEX IF NOT EXISTS departments_is_private_idx ON departments(is_private) WHERE is_private;
