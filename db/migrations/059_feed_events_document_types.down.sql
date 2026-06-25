-- 059_feed_events_document_types.down.sql
-- Revert to the original 054 constraint (lead/deal/client create+edit only).
-- Any document/delete events must be removed first or this will fail.

ALTER TABLE feed_events DROP CONSTRAINT IF EXISTS feed_events_event_type_check;

ALTER TABLE feed_events ADD CONSTRAINT feed_events_event_type_check CHECK (event_type IN (
    'pending_create_lead', 'pending_edit_lead',
    'pending_create_deal', 'pending_edit_deal',
    'pending_create_client', 'pending_edit_client'
));
