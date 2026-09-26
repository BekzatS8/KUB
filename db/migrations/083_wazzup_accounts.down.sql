-- 083_wazzup_accounts.down.sql
-- Уникальность owner_user_id не восстанавливается: если один админ подключил
-- оба аккаунта, индекс не создастся.
DROP INDEX IF EXISTS wazzup_integrations_account_uq;
ALTER TABLE wazzup_integrations DROP COLUMN IF EXISTS account;
