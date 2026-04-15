BEGIN;

ALTER TABLE IF EXISTS client_individual_profiles
    DROP COLUMN IF EXISTS driver_license_issue_date,
    DROP COLUMN IF EXISTS driver_license_expire_date;

COMMIT;
