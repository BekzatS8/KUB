ALTER TABLE chat_read_state
    ADD COLUMN IF NOT EXISTS read_at TIMESTAMPTZ;

UPDATE chat_read_state
SET read_at = NOW()
WHERE last_read_message_id IS NOT NULL
  AND read_at IS NULL;
