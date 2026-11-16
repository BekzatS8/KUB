-- Воронку и отдел не удаляем (в воронке могут быть лиды) — только выключаем маршрут.
DELETE FROM crm_settings WHERE key = 'inbound_to_callcenter';
UPDATE funnels SET is_active = FALSE WHERE code = 'callcenter_default';
