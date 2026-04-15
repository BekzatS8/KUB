BEGIN;

DROP INDEX IF EXISTS company_integrations_company_type_idx;
DROP INDEX IF EXISTS company_integrations_company_idx;
DROP TABLE IF EXISTS company_integrations;

COMMIT;
