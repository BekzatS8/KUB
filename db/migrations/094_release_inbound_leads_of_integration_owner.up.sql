-- 094_release_inbound_leads_of_integration_owner.up.sql
-- ТЗ «доработки CRM КУБ», п.9: «Убрать привязку к Админичу. Все лиды админа
-- вернуть в общий список. Только если админ берёт лид сам, назначив себя
-- ответственным, тогда это его лид».
--
-- Входящие лиды (WhatsApp / Instagram / Telegram / звонки) раньше создавались
-- на владельце интеграции Wazzup — аккаунте админа «Админич» — и «парковались»
-- на нём. Код теперь создаёт их без ответственного; здесь освобождаем уже
-- накопленные (на 09.11.2026 — 820 лидов). Трогаем только лиды владельца
-- интеграции с входящим источником; лиды других админов (их они брали себе
-- лично) и лиды, заведённые вручную, остаются как есть.
--
-- Миграции прогоняются при каждом деплое, поэтому всё делается ОДИН раз — при
-- первом прогоне, когда бэкап-таблицы ещё нет. Иначе лид, который админ потом
-- возьмёт себе сам, освобождался бы заново при каждом деплое.
-- Откат — 094 down: возвращает владельца лидам, которые с тех пор никто не взял.

DO $$
BEGIN
    IF to_regclass('public.leads_owner_backup_094') IS NOT NULL THEN
        RETURN;
    END IF;

    CREATE TABLE leads_owner_backup_094 AS
    SELECT l.id AS lead_id, l.owner_id, NOW() AS backed_up_at
    FROM leads l
    WHERE l.is_archived = FALSE
      AND l.deleted_at IS NULL
      AND COALESCE(l.source, '') IN ('whatsapp', 'instagram', 'telegram', 'binotel')
      AND l.owner_id IN (
          SELECT wi.owner_user_id
          FROM wazzup_integrations wi
          JOIN users u ON u.id = wi.owner_user_id
          WHERE wi.enabled = TRUE
            AND u.role_id = 50
      );

    UPDATE leads l
    SET owner_id = NULL
    FROM leads_owner_backup_094 b
    WHERE l.id = b.lead_id
      AND l.owner_id = b.owner_id;
END $$;
