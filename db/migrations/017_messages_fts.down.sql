DROP INDEX IF EXISTS messages_chat_created_id_desc_idx;
DROP INDEX IF EXISTS messages_search_tsv_gin_idx;

ALTER TABLE messages
    DROP COLUMN IF EXISTS search_tsv;
