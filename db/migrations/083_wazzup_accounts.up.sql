-- 083_wazzup_accounts.up.sql
-- Два аккаунта Wazzup одновременно: основной (User API, apiKey) и дочерний
-- (White Label, Tech Partner API). У каждого своё подключение — свой
-- webhook-токен, свой crmKey и свои каналы с привязкой к филиалам.
--
-- Раньше подключение было «на пользователя» (уникальный owner_user_id), и
-- вызов setup из-под другого админа создавал ещё одно подключение и уводил
-- вебхуки на него. Теперь подключение одно на аккаунт; owner_user_id остаётся
-- как «кто подключил» (от него берётся владелец и филиал входящего лида, если
-- у канала филиал не задан).
--
-- account = NULL — старые подключения до этой миграции. При старте приложение
-- закрепляет за текущим аккаунтом то из них, что реально работает, — каналы,
-- филиалы и вебхук переходят без изменений.
--
-- This migration is re-applied on every deploy, so every statement is idempotent.

ALTER TABLE wazzup_integrations
    ADD COLUMN IF NOT EXISTS account TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS wazzup_integrations_account_uq
    ON wazzup_integrations(account)
    WHERE account IS NOT NULL;

DROP INDEX IF EXISTS wazzup_integrations_owner_user_uq;
