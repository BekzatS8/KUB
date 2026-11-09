-- 098_drive_passwords.down.sql
DROP INDEX IF EXISTS drive_nodes_protected_idx;
ALTER TABLE drive_nodes
    DROP COLUMN IF EXISTS password_set_at,
    DROP COLUMN IF EXISTS password_set_by,
    DROP COLUMN IF EXISTS password_hash;
