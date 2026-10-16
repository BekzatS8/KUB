-- 090_clients_name_from_fio.down.sql
-- Возвращает name/display_name из бэкапа первого прогона.

UPDATE clients c
SET name = b.name,
    display_name = b.display_name
FROM clients_name_backup_090 b
WHERE c.id = b.id;

DROP TABLE IF EXISTS clients_name_backup_090;
