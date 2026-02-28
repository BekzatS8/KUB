ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS search_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', COALESCE(text, ''))) STORED;

CREATE INDEX IF NOT EXISTS messages_search_tsv_gin_idx ON messages USING GIN (search_tsv);
CREATE INDEX IF NOT EXISTS messages_chat_created_id_desc_idx ON messages (chat_id, created_at DESC, id DESC);
