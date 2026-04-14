BEGIN;

ALTER TABLE IF EXISTS client_individual_profiles
    ADD COLUMN IF NOT EXISTS education_level TEXT;

COMMIT;
