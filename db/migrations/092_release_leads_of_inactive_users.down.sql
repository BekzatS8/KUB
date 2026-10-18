-- 092_release_leads_of_inactive_users.down.sql
-- Возвращает ответственных из бэкапа первого прогона — только тем лидам,
-- которые с тех пор никто не взял себе.

UPDATE leads l
SET owner_id = b.owner_id
FROM leads_owner_backup_092 b
WHERE l.id = b.lead_id
  AND l.owner_id IS NULL;

DROP TABLE IF EXISTS leads_owner_backup_092;
