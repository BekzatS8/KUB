BEGIN;

ALTER TABLE IF EXISTS clients
    DROP COLUMN IF EXISTS additional_info,
    DROP COLUMN IF EXISTS diseases_last3_years,
    DROP COLUMN IF EXISTS clinic_name,
    DROP COLUMN IF EXISTS therapist_name,
    DROP COLUMN IF EXISTS driver_license_categories,
    DROP COLUMN IF EXISTS weight,
    DROP COLUMN IF EXISTS height,
    DROP COLUMN IF EXISTS trusted_person,
    DROP COLUMN IF EXISTS relatives_in_destination,
    DROP COLUMN IF EXISTS trips_last5_years,
    DROP COLUMN IF EXISTS job,
    DROP COLUMN IF EXISTS education,
    DROP COLUMN IF EXISTS children_list,
    DROP COLUMN IF EXISTS has_children,
    DROP COLUMN IF EXISTS spouse_contacts,
    DROP COLUMN IF EXISTS spouse_name,
    DROP COLUMN IF EXISTS previous_last_name;

COMMIT;
