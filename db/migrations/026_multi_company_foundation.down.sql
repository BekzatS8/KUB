BEGIN;

DROP INDEX IF EXISTS user_companies_company_idx;
DROP INDEX IF EXISTS user_companies_user_idx;
DROP INDEX IF EXISTS user_companies_primary_uq;

DROP TABLE IF EXISTS user_companies;
DROP TABLE IF EXISTS companies;

COMMIT;
