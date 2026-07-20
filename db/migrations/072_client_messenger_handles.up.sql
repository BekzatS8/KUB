-- Мессенджер-хендлы клиента (Telegram/Instagram) для быстрой связи из карточки.
-- Обратная связь заказчика по видео 17.07.2026: с частью клиентов переписка
-- ведётся в мессенджерах, нужны кнопки прямо в карточке клиента.
-- Миграции переигрываются на каждом деплое — держим идемпотентно.
BEGIN;

ALTER TABLE IF EXISTS clients
    ADD COLUMN IF NOT EXISTS telegram_username  TEXT,
    ADD COLUMN IF NOT EXISTS instagram_username TEXT;

COMMIT;
