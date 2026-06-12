-- Migration 043: add passport_identity as single unified passport field
-- to client_individual_profiles.  Old columns passport_series / passport_number
-- are kept (deprecated) and will be dropped after prod confirmation.

ALTER TABLE client_individual_profiles
    ADD COLUMN IF NOT EXISTS passport_identity TEXT;

-- Backfill: combine existing series + number into the new field.
-- Idempotent: only fills rows where passport_identity is still NULL
-- and at least one of the old fields has a non-empty value.
UPDATE client_individual_profiles
SET passport_identity = TRIM(
        COALESCE(passport_series, '') || ' ' || COALESCE(passport_number, '')
    )
WHERE passport_identity IS NULL
  AND (
        NULLIF(TRIM(COALESCE(passport_series, '')), '') IS NOT NULL
     OR NULLIF(TRIM(COALESCE(passport_number, '')), '') IS NOT NULL
  );

-- TODO: remove old columns after prod confirmation:
--   ALTER TABLE client_individual_profiles
--       DROP COLUMN IF EXISTS passport_series,
--       DROP COLUMN IF EXISTS passport_number;
