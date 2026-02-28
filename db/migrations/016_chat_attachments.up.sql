CREATE TABLE IF NOT EXISTS attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id INT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    message_id INT REFERENCES messages(id) ON DELETE SET NULL,
    uploader_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_name TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    storage_driver TEXT NOT NULL DEFAULT 'local',
    storage_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS attachments_chat_created_idx ON attachments(chat_id, created_at DESC);
CREATE INDEX IF NOT EXISTS attachments_message_idx ON attachments(message_id);
CREATE INDEX IF NOT EXISTS attachments_uploader_created_idx ON attachments(uploader_id, created_at DESC);
