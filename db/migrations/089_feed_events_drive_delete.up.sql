-- 089_feed_events_drive_delete.up.sql
-- Лента: drive_delete — сотрудник удалил файлы или папки в хранилище (они уже
-- в корзине). Одобрить — оставить в корзине, отклонить — восстановить.
--
-- Это финальный constraint с полным списком типов: 059, 060 и 079 ставят свои
-- списки NOT VALID, чтобы повторный прогон миграций при деплое не падал на
-- строках с более новыми типами. Новый тип события — добавлять сюда, а прошлый
-- финальный constraint переводить в NOT VALID.

ALTER TABLE feed_events DROP CONSTRAINT IF EXISTS feed_events_event_type_check;

ALTER TABLE feed_events ADD CONSTRAINT feed_events_event_type_check CHECK (event_type IN (
    'pending_create_lead', 'pending_edit_lead', 'pending_delete_lead',
    'pending_create_deal', 'pending_edit_deal', 'pending_delete_deal',
    'pending_create_client', 'pending_edit_client', 'pending_delete_client',
    'pending_create_document', 'pending_edit_document', 'pending_delete_document',
    'pending_send_document', 'pending_review_document',
    'drive_delete'
));
