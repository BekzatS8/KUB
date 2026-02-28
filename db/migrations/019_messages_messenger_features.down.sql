DROP INDEX IF EXISTS message_favorites_user_created_idx;
DROP TABLE IF EXISTS message_favorites;

DROP INDEX IF EXISTS chat_pins_chat_pinned_idx;
DROP TABLE IF EXISTS chat_pins;

ALTER TABLE messages
    DROP COLUMN IF EXISTS delete_reason,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS edited_at;
