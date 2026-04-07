CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_user_id INT REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    entity_type TEXT,
    entity_id TEXT,
    ip INET,
    user_agent TEXT,
    meta JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS audit_logs_created_at_idx ON audit_logs(created_at);
CREATE INDEX IF NOT EXISTS audit_logs_actor_idx ON audit_logs(actor_user_id);
CREATE INDEX IF NOT EXISTS audit_logs_action_idx ON audit_logs(action);
