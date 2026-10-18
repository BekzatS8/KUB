-- 093_release_deals_of_inactive_users.down.sql
-- Возвращает ответственных из бэкапа — только сделкам, которые никто не взял.
-- owner_id остаётся необязательным: после работы кода могут быть и другие
-- сделки без ответственного.

UPDATE deals d
SET owner_id = b.owner_id
FROM deals_owner_backup_093 b
WHERE d.id = b.deal_id
  AND d.owner_id IS NULL;

DROP TABLE IF EXISTS deals_owner_backup_093;
