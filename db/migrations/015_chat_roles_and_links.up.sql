ALTER TABLE chat_members
    ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'member',
    ADD COLUMN IF NOT EXISTS joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE chats c
SET creator_id = src.user_id
FROM (
    SELECT chat_id, MIN(user_id) AS user_id
    FROM chat_members
    GROUP BY chat_id
) src
WHERE c.id = src.chat_id
  AND c.creator_id IS NULL;

UPDATE chat_members cm
SET role = 'owner'
FROM chats c
WHERE cm.chat_id = c.id
  AND cm.user_id = c.creator_id;

ALTER TABLE chats
    ADD COLUMN IF NOT EXISTS client_id UUID,
    ADD COLUMN IF NOT EXISTS deal_id UUID,
    ADD COLUMN IF NOT EXISTS lead_id UUID;

CREATE INDEX IF NOT EXISTS chat_members_chat_idx ON chat_members(chat_id);
CREATE INDEX IF NOT EXISTS chat_members_chat_role_idx ON chat_members(chat_id, role);

