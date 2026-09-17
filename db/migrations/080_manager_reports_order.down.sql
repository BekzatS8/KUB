-- 080_manager_reports_order.down.sql

DROP INDEX IF EXISTS manager_reports_user_position_idx;

ALTER TABLE manager_reports DROP COLUMN IF EXISTS position;
