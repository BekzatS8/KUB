ALTER TABLE user_approval_requests DROP CONSTRAINT IF EXISTS user_approval_requests_action_check;
ALTER TABLE user_approval_requests ADD CONSTRAINT user_approval_requests_action_check CHECK (action IN ('create', 'delete'));
