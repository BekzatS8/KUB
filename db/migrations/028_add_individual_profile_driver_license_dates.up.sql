BEGIN;

ALTER TABLE IF EXISTS client_individual_profiles
    ADD COLUMN IF NOT EXISTS driver_license_issue_date DATE,
    ADD COLUMN IF NOT EXISTS driver_license_expire_date DATE;

COMMIT;
