BEGIN;

ALTER TABLE IF EXISTS clients
    DROP COLUMN IF EXISTS telegram_username,
    DROP COLUMN IF EXISTS instagram_username;

COMMIT;
