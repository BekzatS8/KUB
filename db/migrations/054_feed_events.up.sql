-- 054_feed_events.up.sql
-- Pending-approval events from non-admin roles (visa, sales, etc.)
-- that require an admin to approve/reject before the action is applied.

CREATE TABLE IF NOT EXISTS feed_events (
    id             SERIAL PRIMARY KEY,
    event_type     TEXT NOT NULL CHECK (event_type IN (
                     'pending_create_lead', 'pending_edit_lead',
                     'pending_create_deal', 'pending_edit_deal',
                     'pending_create_client', 'pending_edit_client'
                   )),
    status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    requester_id   INT  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    requester_name TEXT NOT NULL DEFAULT '',
    payload        JSONB NOT NULL DEFAULT '{}',
    resource_id    INT,
    reject_reason  TEXT,
    reviewer_id    INT  REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at    TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_feed_events_status      ON feed_events(status);
CREATE INDEX IF NOT EXISTS idx_feed_events_requester   ON feed_events(requester_id);
CREATE INDEX IF NOT EXISTS idx_feed_events_created_at  ON feed_events(created_at DESC);
