DROP INDEX IF EXISTS chat_members_chat_role_idx;
DROP INDEX IF EXISTS chat_members_chat_idx;

ALTER TABLE chats
    DROP COLUMN IF EXISTS lead_id,
    DROP COLUMN IF EXISTS deal_id,
    DROP COLUMN IF EXISTS client_id;

ALTER TABLE chat_members
    DROP COLUMN IF EXISTS joined_at,
    DROP COLUMN IF EXISTS role;
