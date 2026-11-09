-- 095_operator_role_lead_brief.down.sql
-- Операторов переводим в МОП, иначе удалить роль не даст внешний ключ.

DROP TABLE IF EXISTS lead_history;

DROP INDEX IF EXISTS leads_handed_to_sales_by_idx;

ALTER TABLE leads
    DROP COLUMN IF EXISTS merged_into_id,
    DROP COLUMN IF EXISTS handed_to_sales_by,
    DROP COLUMN IF EXISTS handed_to_sales_at,
    DROP COLUMN IF EXISTS brief_comment,
    DROP COLUMN IF EXISTS brief_age,
    DROP COLUMN IF EXISTS brief_city,
    DROP COLUMN IF EXISTS brief_visa_type,
    DROP COLUMN IF EXISTS brief_country,
    DROP COLUMN IF EXISTS brief_full_name;

UPDATE users SET role_id = 10 WHERE role_id = 95;
DELETE FROM role_permissions WHERE role_id = 95;
DELETE FROM roles WHERE id = 95;
UPDATE users SET department_id = NULL
WHERE department_id IN (SELECT id FROM departments WHERE code = 'call_center');
DELETE FROM departments WHERE code = 'call_center';
