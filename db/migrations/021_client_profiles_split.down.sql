BEGIN;
DROP TABLE IF EXISTS client_legal_profiles;
DROP TABLE IF EXISTS client_individual_profiles;
ALTER TABLE clients
    DROP COLUMN IF EXISTS display_name,
    DROP COLUMN IF EXISTS primary_phone,
    DROP COLUMN IF EXISTS primary_email,
    DROP COLUMN IF EXISTS updated_at;
COMMIT;
