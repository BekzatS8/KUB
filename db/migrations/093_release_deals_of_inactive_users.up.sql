-- 093_release_deals_of_inactive_users.up.sql
-- Сделки уволенных сотрудников — как их лиды (092): остаются на этапах, но
-- без ответственного; их берёт себе любой сотрудник филиала. Для этого
-- deals.owner_id становится необязательным (как leads.owner_id в 058).
--
-- «Не работает» — как в проверке доступа (GetAccountStatus). Архивные и
-- удалённые сделки не трогаем. Бэкап — один раз, при первом прогоне.

ALTER TABLE deals ALTER COLUMN owner_id DROP NOT NULL;

CREATE TABLE IF NOT EXISTS deals_owner_backup_093 AS
SELECT d.id AS deal_id, d.owner_id, NOW() AS backed_up_at
FROM deals d
JOIN users u ON u.id = d.owner_id
WHERE d.is_archived = FALSE
  AND d.deleted_at IS NULL
  AND (COALESCE(u.is_active, TRUE) = FALSE OR COALESCE(u.is_verified, FALSE) = FALSE);

UPDATE deals d
SET owner_id = NULL
FROM users u
WHERE u.id = d.owner_id
  AND d.is_archived = FALSE
  AND d.deleted_at IS NULL
  AND (COALESCE(u.is_active, TRUE) = FALSE OR COALESCE(u.is_verified, FALSE) = FALSE);
