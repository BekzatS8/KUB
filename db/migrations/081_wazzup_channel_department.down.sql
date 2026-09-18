-- 081_wazzup_channel_department.down.sql

DROP INDEX IF EXISTS departments_is_private_idx;
DROP INDEX IF EXISTS wazzup_channels_department_id_idx;

ALTER TABLE departments DROP COLUMN IF EXISTS is_private;
ALTER TABLE wazzup_channels DROP COLUMN IF EXISTS department_id;
