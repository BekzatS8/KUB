-- 079_feed_events_send_review.up.sql
-- Расширяем feed_events.event_type: отправка документа на ПОДПИСЬ и на ПРОВЕРКУ
-- через одобрение в Ленте. Без этих значений INSERT падал на
-- feed_events_event_type_check — заявки менеджера не попадали в Ленту админа.
-- Идемпотентно: DROP IF EXISTS + пересоздание.

ALTER TABLE feed_events DROP CONSTRAINT IF EXISTS feed_events_event_type_check;

ALTER TABLE feed_events ADD CONSTRAINT feed_events_event_type_check CHECK (event_type IN (
    'pending_create_lead', 'pending_edit_lead', 'pending_delete_lead',
    'pending_create_deal', 'pending_edit_deal', 'pending_delete_deal',
    'pending_create_client', 'pending_edit_client', 'pending_delete_client',
    'pending_create_document', 'pending_edit_document', 'pending_delete_document',
    'pending_send_document', 'pending_review_document'
));
