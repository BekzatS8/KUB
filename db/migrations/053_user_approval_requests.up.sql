-- Запросы на создание/удаление пользователей от юриста (требуют подтверждения админа)
CREATE TABLE user_approval_requests (
    id              SERIAL PRIMARY KEY,
    requester_id    INT         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action          TEXT        NOT NULL CHECK (action IN ('create', 'delete')),
    target_user_id  INT         REFERENCES users(id) ON DELETE CASCADE,
    request_data    JSONB,
    status          TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    reviewer_id     INT         REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_uar_status     ON user_approval_requests(status);
CREATE INDEX idx_uar_requester  ON user_approval_requests(requester_id);
CREATE INDEX idx_uar_created_at ON user_approval_requests(created_at DESC);
