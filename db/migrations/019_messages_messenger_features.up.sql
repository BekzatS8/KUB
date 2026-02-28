ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS edited_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by INT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS delete_reason TEXT;

CREATE TABLE IF NOT EXISTS chat_pins (
    chat_id INT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    message_id INT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    pinned_by INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pinned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id, message_id)
);

CREATE INDEX IF NOT EXISTS chat_pins_chat_pinned_idx ON chat_pins(chat_id, pinned_at DESC);

CREATE TABLE IF NOT EXISTS message_favorites (
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id INT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, message_id)
);

CREATE INDEX IF NOT EXISTS message_favorites_user_created_idx ON message_favorites(user_id, created_at DESC);
