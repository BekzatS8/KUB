-- 100_close_leads_with_deals.up.sql
-- Замечания заказчика 13.11.2026: по лиду оформили сделку, а лид так и висит
-- на доске «Новой заявкой» / «В работе» — на одного человека копятся дубли.
-- Код теперь закрывает лид («Конвертирован») при создании сделки с этим лидом;
-- здесь закрываем уже накопленные (на 16.11.2026 — 15 лидов).
--
-- Миграции прогоняются при каждом деплое — всё делается ОДИН раз, пока нет
-- бэкап-таблицы. Откат — 100 down: возвращает прежний статус.

DO $$
BEGIN
    IF to_regclass('public.leads_status_backup_100') IS NOT NULL THEN
        RETURN;
    END IF;

    CREATE TABLE leads_status_backup_100 AS
    SELECT DISTINCT l.id AS lead_id, l.status, NOW() AS backed_up_at
    FROM leads l
    JOIN deals d ON d.lead_id = l.id AND d.deleted_at IS NULL
    WHERE l.deleted_at IS NULL
      AND COALESCE(l.status, 'new') NOT IN ('converted', 'cancelled');

    UPDATE leads SET status = 'converted'
    WHERE id IN (SELECT lead_id FROM leads_status_backup_100);
END $$;
