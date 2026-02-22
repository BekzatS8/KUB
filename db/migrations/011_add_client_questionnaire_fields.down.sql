BEGIN;

DROP INDEX IF EXISTS client_files_primary_unique_idx;
DROP INDEX IF EXISTS client_files_client_id_category_idx;
DROP INDEX IF EXISTS client_files_client_id_idx;

DROP TABLE IF EXISTS client_files;

ALTER TABLE IF EXISTS clients
    DROP COLUMN IF EXISTS passport_expire_date,
    DROP COLUMN IF EXISTS passport_issue_date,
    DROP COLUMN IF EXISTS marital_status,
    DROP COLUMN IF EXISTS sex,
    DROP COLUMN IF EXISTS citizenship,
    DROP COLUMN IF EXISTS birth_place,
    DROP COLUMN IF EXISTS birth_date,
    DROP COLUMN IF EXISTS trip_purpose,
    DROP COLUMN IF EXISTS country;

DROP INDEX IF EXISTS clients_country_idx;

COMMIT;
