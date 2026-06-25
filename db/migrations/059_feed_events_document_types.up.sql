-- 059_feed_events_document_types.up.sql
-- Extend feed_events.event_type to cover document approval requests from the
-- quality-control department (ОКК): creating/editing a document is submitted to
-- the admin feed and applied with admin credentials on approval.
--
-- This rewrites the CHECK constraint to the full current set of event types,
-- including the delete_* types defined in the Go model that were never added to
-- the original 054 constraint.

ALTER TABLE feed_events DROP CONSTRAINT IF EXISTS feed_events_event_type_check;

ALTER TABLE feed_events ADD CONSTRAINT feed_events_event_type_check CHECK (event_type IN (
    'pending_create_lead', 'pending_edit_lead', 'pending_delete_lead',
    'pending_create_deal', 'pending_edit_deal', 'pending_delete_deal',
    'pending_create_client', 'pending_edit_client', 'pending_delete_client',
    'pending_create_document', 'pending_edit_document'
));
