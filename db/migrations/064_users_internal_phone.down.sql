-- 064_users_internal_phone.down.sql
DROP INDEX IF EXISTS users_internal_phone_idx;
ALTER TABLE users DROP COLUMN IF EXISTS internal_phone;
