-- 086_drive_group_shares.up.sql
-- Доступ в хранилище не только сотруднику, но и группе: филиалу, отделу или
-- всем сразу. Группа проверяется в момент доступа, поэтому сотрудник, которого
-- позже добавили в филиал или отдел, получает доступ сам.
--
-- target:
--   user       — один сотрудник (user_id);
--   branch     — все сотрудники филиала (branch_id);
--   department — все сотрудники отдела (department_id);
--   all        — все сотрудники.
--
-- This migration is re-applied on every deploy, so every statement is idempotent.

ALTER TABLE drive_shares ADD COLUMN IF NOT EXISTS target TEXT NOT NULL DEFAULT 'user';
ALTER TABLE drive_shares ADD COLUMN IF NOT EXISTS branch_id INT REFERENCES branches(id) ON DELETE CASCADE;
ALTER TABLE drive_shares ADD COLUMN IF NOT EXISTS department_id INT REFERENCES departments(id) ON DELETE CASCADE;
ALTER TABLE drive_shares ALTER COLUMN user_id DROP NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'drive_shares_target_check') THEN
        ALTER TABLE drive_shares ADD CONSTRAINT drive_shares_target_check CHECK (
            (target = 'user'       AND user_id IS NOT NULL AND branch_id IS NULL AND department_id IS NULL) OR
            (target = 'branch'     AND branch_id IS NOT NULL AND user_id IS NULL AND department_id IS NULL) OR
            (target = 'department' AND department_id IS NOT NULL AND user_id IS NULL AND branch_id IS NULL) OR
            (target = 'all'        AND user_id IS NULL AND branch_id IS NULL AND department_id IS NULL)
        );
    END IF;
END $$;

-- Одна запись на группу и узел: повторная выдача обновляет срок.
CREATE UNIQUE INDEX IF NOT EXISTS drive_shares_branch_uniq
    ON drive_shares (node_id, branch_id) WHERE target = 'branch';
CREATE UNIQUE INDEX IF NOT EXISTS drive_shares_department_uniq
    ON drive_shares (node_id, department_id) WHERE target = 'department';
CREATE UNIQUE INDEX IF NOT EXISTS drive_shares_all_uniq
    ON drive_shares (node_id) WHERE target = 'all';
