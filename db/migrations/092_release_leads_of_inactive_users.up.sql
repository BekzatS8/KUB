-- 092_release_leads_of_inactive_users.up.sql
-- Лиды уволенных сотрудников (заблокирован, не подтверждён или удалён)
-- остаются на своих этапах, но без ответственного — их видит весь филиал и
-- забирает себе («Взять себе» или перенос карточки). Новые блокировки
-- освобождает код (UserService), здесь — те, кого заблокировали раньше.
--
-- «Не работает» — как в проверке доступа (GetAccountStatus):
-- COALESCE(is_active, TRUE) = FALSE ИЛИ COALESCE(is_verified, FALSE) = FALSE.
-- Архивные и удалённые лиды не трогаем — там владелец нужен для истории.
--
-- Миграции прогоняются при каждом деплое: бэкап создаётся один раз (первый
-- прогон), UPDATE затрагивает только ещё не освобождённые лиды.

CREATE TABLE IF NOT EXISTS leads_owner_backup_092 AS
SELECT l.id AS lead_id, l.owner_id, NOW() AS backed_up_at
FROM leads l
JOIN users u ON u.id = l.owner_id
WHERE l.is_archived = FALSE
  AND l.deleted_at IS NULL
  AND (COALESCE(u.is_active, TRUE) = FALSE OR COALESCE(u.is_verified, FALSE) = FALSE);

UPDATE leads l
SET owner_id = NULL
FROM users u
WHERE u.id = l.owner_id
  AND l.is_archived = FALSE
  AND l.deleted_at IS NULL
  AND (COALESCE(u.is_active, TRUE) = FALSE OR COALESCE(u.is_verified, FALSE) = FALSE);
