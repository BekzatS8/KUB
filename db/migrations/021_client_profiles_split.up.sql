BEGIN;

ALTER TABLE clients
    ADD COLUMN IF NOT EXISTS display_name TEXT,
    ADD COLUMN IF NOT EXISTS primary_phone VARCHAR(50),
    ADD COLUMN IF NOT EXISTS primary_email VARCHAR(255),
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE clients
SET display_name = COALESCE(NULLIF(display_name, ''), NULLIF(name, ''), CONCAT_WS(' ', NULLIF(last_name, ''), NULLIF(first_name, ''), NULLIF(middle_name, ''))),
    primary_phone = COALESCE(NULLIF(primary_phone, ''), NULLIF(phone, '')),
    primary_email = COALESCE(NULLIF(primary_email, ''), NULLIF(email, '')),
    updated_at = COALESCE(updated_at, created_at, NOW());

CREATE TABLE IF NOT EXISTS client_individual_profiles (
    client_id INT PRIMARY KEY REFERENCES clients(id) ON DELETE CASCADE,
    last_name TEXT,
    first_name TEXT,
    middle_name TEXT,
    iin VARCHAR(20),
    id_number TEXT,
    passport_series TEXT,
    passport_number TEXT,
    registration_address TEXT,
    actual_address TEXT,
    country TEXT,
    trip_purpose TEXT,
    birth_date DATE,
    birth_place TEXT,
    citizenship TEXT,
    sex VARCHAR(20),
    marital_status VARCHAR(50),
    passport_issue_date DATE,
    passport_expire_date DATE,
    previous_last_name TEXT,
    spouse_name TEXT,
    spouse_contacts TEXT,
    has_children BOOLEAN,
    children_list JSONB,
    education TEXT,
    job TEXT,
    trips_last5_years TEXT,
    relatives_in_destination TEXT,
    trusted_person TEXT,
    height SMALLINT,
    weight SMALLINT,
    driver_license_categories JSONB,
    therapist_name TEXT,
    clinic_name TEXT,
    diseases_last3_years TEXT,
    additional_info TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS client_legal_profiles (
    client_id INT PRIMARY KEY REFERENCES clients(id) ON DELETE CASCADE,
    company_name TEXT NOT NULL,
    bin VARCHAR(20),
    legal_form TEXT,
    director_full_name TEXT,
    contact_person_name TEXT,
    contact_person_position TEXT,
    contact_person_phone VARCHAR(50),
    contact_person_email VARCHAR(255),
    legal_address TEXT,
    actual_address TEXT,
    bank_name TEXT,
    iban TEXT,
    bik TEXT,
    kbe TEXT,
    tax_regime TEXT,
    website TEXT,
    industry TEXT,
    company_size TEXT,
    additional_info TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS client_individual_profiles_iin_uq
    ON client_individual_profiles (iin)
    WHERE iin IS NOT NULL AND iin <> '';

CREATE UNIQUE INDEX IF NOT EXISTS client_legal_profiles_bin_uq
    ON client_legal_profiles (bin)
    WHERE bin IS NOT NULL AND bin <> '';

INSERT INTO client_individual_profiles (
    client_id, last_name, first_name, middle_name, iin, id_number, passport_series, passport_number,
    registration_address, actual_address, country, trip_purpose, birth_date, birth_place, citizenship,
    sex, marital_status, passport_issue_date, passport_expire_date, previous_last_name, spouse_name,
    spouse_contacts, has_children, children_list, education, job, trips_last5_years,
    relatives_in_destination, trusted_person, height, weight, driver_license_categories,
    therapist_name, clinic_name, diseases_last3_years, additional_info
)
SELECT
    id, last_name, first_name, middle_name, iin, id_number, passport_series, passport_number,
    registration_address, actual_address, country, trip_purpose, birth_date, birth_place, citizenship,
    sex, marital_status, passport_issue_date, passport_expire_date, previous_last_name, spouse_name,
    spouse_contacts, has_children, children_list, education, job, trips_last5_years,
    relatives_in_destination, trusted_person, height, weight, driver_license_categories,
    therapist_name, clinic_name, diseases_last3_years, additional_info
FROM clients
WHERE client_type = 'individual'
ON CONFLICT (client_id) DO NOTHING;

INSERT INTO client_legal_profiles (
    client_id, company_name, bin, contact_person_phone, contact_person_email, legal_address, actual_address, additional_info
)
SELECT
    id,
    COALESCE(NULLIF(name, ''), display_name),
    NULLIF(bin_iin, ''),
    NULLIF(phone, ''),
    NULLIF(email, ''),
    NULLIF(address, ''),
    NULLIF(actual_address, ''),
    NULLIF(contact_info, '')
FROM clients
WHERE client_type = 'legal'
ON CONFLICT (client_id) DO NOTHING;

COMMIT;
