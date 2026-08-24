BEGIN;

-- Откатываем только структуру. Бэкфилл branch_id у лидов/клиентов не
-- отменяем — это восстановленные данные, потеря которых опаснее отката колонки.
DROP INDEX IF EXISTS wazzup_channels_branch_id_idx;

ALTER TABLE wazzup_channels
    DROP COLUMN IF EXISTS branch_id;

COMMIT;
