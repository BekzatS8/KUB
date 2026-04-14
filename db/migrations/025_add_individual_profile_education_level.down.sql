BEGIN;

ALTER TABLE IF EXISTS client_individual_profiles
    DROP COLUMN IF EXISTS education_level;

COMMIT;
