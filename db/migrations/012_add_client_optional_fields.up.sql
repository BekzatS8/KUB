BEGIN;

ALTER TABLE IF EXISTS clients
    ADD COLUMN IF NOT EXISTS previous_last_name        TEXT,
    ADD COLUMN IF NOT EXISTS spouse_name               TEXT,
    ADD COLUMN IF NOT EXISTS spouse_contacts           TEXT,
    ADD COLUMN IF NOT EXISTS has_children              BOOLEAN,
    ADD COLUMN IF NOT EXISTS children_list             JSONB,
    ADD COLUMN IF NOT EXISTS education                 TEXT,
    ADD COLUMN IF NOT EXISTS job                       TEXT,
    ADD COLUMN IF NOT EXISTS trips_last5_years         TEXT,
    ADD COLUMN IF NOT EXISTS relatives_in_destination  TEXT,
    ADD COLUMN IF NOT EXISTS trusted_person            TEXT,
    ADD COLUMN IF NOT EXISTS height                    SMALLINT,
    ADD COLUMN IF NOT EXISTS weight                    SMALLINT,
    ADD COLUMN IF NOT EXISTS driver_license_categories JSONB,
    ADD COLUMN IF NOT EXISTS therapist_name            TEXT,
    ADD COLUMN IF NOT EXISTS clinic_name               TEXT,
    ADD COLUMN IF NOT EXISTS diseases_last3_years      TEXT,
    ADD COLUMN IF NOT EXISTS additional_info           TEXT;

COMMIT;
