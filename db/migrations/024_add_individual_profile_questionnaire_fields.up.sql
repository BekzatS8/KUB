BEGIN;

ALTER TABLE IF EXISTS client_individual_profiles
    ADD COLUMN IF NOT EXISTS specialty TEXT,
    ADD COLUMN IF NOT EXISTS trusted_person_phone VARCHAR(50),
    ADD COLUMN IF NOT EXISTS driver_license_number TEXT,
    ADD COLUMN IF NOT EXISTS education_institution_name TEXT,
    ADD COLUMN IF NOT EXISTS education_institution_address TEXT,
    ADD COLUMN IF NOT EXISTS position TEXT,
    ADD COLUMN IF NOT EXISTS visas_received TEXT,
    ADD COLUMN IF NOT EXISTS visa_refusals TEXT;

COMMIT;
