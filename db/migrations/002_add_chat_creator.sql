BEGIN;

ALTER TABLE chats
    ADD COLUMN IF NOT EXISTS creator_id INT;

-- заполнить creator_id для старых чатов
UPDATE chats c
SET creator_id = x.min_user
FROM (
         SELECT chat_id, MIN(user_id) AS min_user
         FROM chat_members
         GROUP BY chat_id
     ) x
WHERE c.id = x.chat_id AND c.creator_id IS NULL;

-- constraint "IF NOT EXISTS" делаем через DO-block
DO $$
    BEGIN
        ALTER TABLE chats
            ADD CONSTRAINT chats_creator_fk
                FOREIGN KEY (creator_id) REFERENCES users(id) ON DELETE SET NULL;
    EXCEPTION
        WHEN duplicate_object THEN
            NULL;
    END $$;

COMMIT;
