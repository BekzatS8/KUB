DELETE FROM drive_nodes WHERE deleted_at IS NOT NULL;
DROP INDEX IF EXISTS drive_nodes_live_name_uniq;
DROP INDEX IF EXISTS drive_nodes_trash_root_idx;
ALTER TABLE drive_nodes DROP COLUMN IF EXISTS trash_root_id;
ALTER TABLE drive_nodes DROP COLUMN IF EXISTS deleted_by;
ALTER TABLE drive_nodes DROP COLUMN IF EXISTS deleted_at;
CREATE UNIQUE INDEX IF NOT EXISTS drive_nodes_name_uniq
    ON drive_nodes (COALESCE(parent_id, 0), lower(name));
