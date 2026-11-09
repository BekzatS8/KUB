-- 094_release_inbound_leads_of_integration_owner.down.sql
-- Возвращает владельца из бэкапа первого прогона — только тем лидам, которые
-- с тех пор никто не взял себе.

DO $$
BEGIN
    IF to_regclass('public.leads_owner_backup_094') IS NULL THEN
        RETURN;
    END IF;

    UPDATE leads l
    SET owner_id = b.owner_id
    FROM leads_owner_backup_094 b
    WHERE l.id = b.lead_id
      AND l.owner_id IS NULL;

    DROP TABLE leads_owner_backup_094;
END $$;
