-- 090_clients_name_from_fio.up.sql
-- Имя физлица в таблице клиентов (name/display_name) — это ФИО из анкеты.
-- Раньше name собирался из ФИО только при создании без имени и потом не
-- обновлялся, поэтому у части клиентов в таблице осталась латиница из
-- паспорта или капс из лида, а в карточке — правильное ФИО.
--
-- Миграции прогоняются при каждом деплое: бэкап создаётся один раз (первый
-- прогон), UPDATE трогает только расходящиеся строки.

CREATE TABLE IF NOT EXISTS clients_name_backup_090 AS
SELECT c.id, c.name, c.display_name, NOW() AS backed_up_at
FROM clients c
LEFT JOIN client_individual_profiles ip ON ip.client_id = c.id
WHERE COALESCE(NULLIF(c.client_type, ''), 'individual') = 'individual'
  AND COALESCE(NULLIF(btrim(ip.last_name), ''), NULLIF(btrim(c.last_name), ''),
               NULLIF(btrim(ip.first_name), ''), NULLIF(btrim(c.first_name), '')) IS NOT NULL
  AND (c.name IS DISTINCT FROM CONCAT_WS(' ',
         COALESCE(NULLIF(btrim(ip.last_name), ''), NULLIF(btrim(c.last_name), '')),
         COALESCE(NULLIF(btrim(ip.first_name), ''), NULLIF(btrim(c.first_name), '')),
         COALESCE(NULLIF(btrim(ip.middle_name), ''), NULLIF(btrim(c.middle_name), '')))
    OR c.display_name IS DISTINCT FROM c.name);

WITH fio AS (
    SELECT c.id,
           CONCAT_WS(' ',
               COALESCE(NULLIF(btrim(ip.last_name), ''), NULLIF(btrim(c.last_name), '')),
               COALESCE(NULLIF(btrim(ip.first_name), ''), NULLIF(btrim(c.first_name), '')),
               COALESCE(NULLIF(btrim(ip.middle_name), ''), NULLIF(btrim(c.middle_name), ''))) AS full_name
    FROM clients c
    LEFT JOIN client_individual_profiles ip ON ip.client_id = c.id
    WHERE COALESCE(NULLIF(c.client_type, ''), 'individual') = 'individual'
      AND COALESCE(NULLIF(btrim(ip.last_name), ''), NULLIF(btrim(c.last_name), ''),
                   NULLIF(btrim(ip.first_name), ''), NULLIF(btrim(c.first_name), '')) IS NOT NULL
)
UPDATE clients c
SET name = fio.full_name,
    display_name = fio.full_name
FROM fio
WHERE c.id = fio.id
  AND (c.name IS DISTINCT FROM fio.full_name OR c.display_name IS DISTINCT FROM fio.full_name);
