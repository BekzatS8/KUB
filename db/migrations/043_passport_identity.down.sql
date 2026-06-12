-- Rollback migration 043
ALTER TABLE client_individual_profiles
    DROP COLUMN IF EXISTS passport_identity;
