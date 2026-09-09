-- 079_feed_events_send_review.down.sql
-- Откат к списку из 060 (без send/review типов).

ALTER TABLE feed_events DROP CONSTRAINT IF EXISTS feed_events_event_type_check;

ALTER TABLE feed_events ADD CONSTRAINT feed_events_event_type_check CHECK (event_type IN (
    'pending_create_lead', 'pending_edit_lead', 'pending_delete_lead',
    'pending_create_deal', 'pending_edit_deal', 'pending_delete_deal',
    'pending_create_client', 'pending_edit_client', 'pending_delete_client',
    'pending_create_document', 'pending_edit_document', 'pending_delete_document'
));
