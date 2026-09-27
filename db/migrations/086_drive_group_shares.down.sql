DROP INDEX IF EXISTS drive_shares_all_uniq;
DROP INDEX IF EXISTS drive_shares_department_uniq;
DROP INDEX IF EXISTS drive_shares_branch_uniq;
DELETE FROM drive_shares WHERE target <> 'user';
ALTER TABLE drive_shares DROP CONSTRAINT IF EXISTS drive_shares_target_check;
ALTER TABLE drive_shares ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE drive_shares DROP COLUMN IF EXISTS department_id;
ALTER TABLE drive_shares DROP COLUMN IF EXISTS branch_id;
ALTER TABLE drive_shares DROP COLUMN IF EXISTS target;
