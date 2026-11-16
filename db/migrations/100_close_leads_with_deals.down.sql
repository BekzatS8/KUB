-- Возвращает лидам статус, бывший до 100 up (только тем, что всё ещё «converted»).
DO $$
BEGIN
    IF to_regclass('public.leads_status_backup_100') IS NULL THEN
        RETURN;
    END IF;
    UPDATE leads l SET status = b.status
    FROM leads_status_backup_100 b
    WHERE l.id = b.lead_id AND l.status = 'converted';
    DROP TABLE leads_status_backup_100;
END $$;
