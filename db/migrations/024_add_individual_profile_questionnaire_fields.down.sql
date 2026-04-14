BEGIN;

ALTER TABLE IF EXISTS client_individual_profiles
    DROP COLUMN IF EXISTS visa_refusals,
    DROP COLUMN IF EXISTS visas_received,
    DROP COLUMN IF EXISTS position,
    DROP COLUMN IF EXISTS education_institution_address,
    DROP COLUMN IF EXISTS education_institution_name,
    DROP COLUMN IF EXISTS driver_license_number,
    DROP COLUMN IF EXISTS trusted_person_phone,
    DROP COLUMN IF EXISTS specialty;

COMMIT;
