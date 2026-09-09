-- 060_feed_events_document_delete.up.sql
-- Extend feed_events.event_type to cover document DELETE approval requests.
-- HR (отдел кадров) cannot delete documents directly — the request is submitted
-- to the admin feed and applied with admin credentials on approval.
--
-- NB: как и в 059 — ставим NOT VALID, чтобы повторный прогон не падал на строках
-- с более новыми типами (pending_send_document / pending_review_document из 079).
-- Финальный валидный constraint с полным списком выставляет 079.

ALTER TABLE feed_events DROP CONSTRAINT IF EXISTS feed_events_event_type_check;

ALTER TABLE feed_events ADD CONSTRAINT feed_events_event_type_check CHECK (event_type IN (
    'pending_create_lead', 'pending_edit_lead', 'pending_delete_lead',
    'pending_create_deal', 'pending_edit_deal', 'pending_delete_deal',
    'pending_create_client', 'pending_edit_client', 'pending_delete_client',
    'pending_create_document', 'pending_edit_document', 'pending_delete_document'
)) NOT VALID;
