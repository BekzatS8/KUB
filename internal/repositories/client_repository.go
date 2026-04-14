package repositories

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"turcompany/internal/models"
)

type ClientRepository struct{ db *sql.DB }

func NewClientRepository(db *sql.DB) *ClientRepository { return &ClientRepository{db: db} }

type ClientListFilter struct {
	Query           string
	ClientType      string
	HasDeals        *bool
	DealStatusGroup string
	SortBy          string
	Order           string
}

type clientRowScanner interface{ Scan(dest ...any) error }

const clientSelect = `
SELECT
	c.id,
	c.owner_id,
	COALESCE(NULLIF(c.client_type, ''), 'individual') AS client_type,
	COALESCE(NULLIF(c.display_name, ''), NULLIF(c.name, ''), '') AS display_name,
	COALESCE(NULLIF(c.primary_phone, ''), NULLIF(c.phone, ''), '') AS primary_phone,
	COALESCE(NULLIF(c.primary_email, ''), NULLIF(c.email, ''), '') AS primary_email,
	COALESCE(c.address, '') AS address,
	COALESCE(c.contact_info, '') AS contact_info,
	c.created_at,
	COALESCE(c.updated_at, c.created_at) AS updated_at,
	c.is_archived,
	c.archived_at,
	c.archived_by,
	COALESCE(c.archive_reason, '') AS archive_reason,
	COALESCE(ip.last_name, c.last_name, ''), COALESCE(ip.first_name, c.first_name, ''), COALESCE(ip.middle_name, c.middle_name, ''), COALESCE(ip.iin, c.iin, ''), COALESCE(ip.id_number, c.id_number, ''), COALESCE(ip.passport_series, c.passport_series, ''), COALESCE(ip.passport_number, c.passport_number, ''),
	COALESCE(ip.registration_address, c.registration_address, ''), COALESCE(ip.actual_address, c.actual_address, ''), COALESCE(ip.country, c.country, ''), COALESCE(ip.trip_purpose, c.trip_purpose, ''), COALESCE(ip.birth_date, c.birth_date), COALESCE(ip.birth_place, c.birth_place, ''),
	COALESCE(ip.citizenship, c.citizenship, ''), COALESCE(ip.sex, c.sex, ''), COALESCE(ip.marital_status, c.marital_status, ''), COALESCE(ip.passport_issue_date, c.passport_issue_date), COALESCE(ip.passport_expire_date, c.passport_expire_date),
	COALESCE(ip.previous_last_name, c.previous_last_name, ''), COALESCE(ip.spouse_name, c.spouse_name, ''), COALESCE(ip.spouse_contacts, c.spouse_contacts, ''), COALESCE(ip.has_children, c.has_children), COALESCE(ip.children_list, c.children_list),
	COALESCE(ip.education, c.education, ''), COALESCE(ip.job, c.job, ''), COALESCE(ip.trips_last5_years, c.trips_last5_years, ''), COALESCE(ip.relatives_in_destination, c.relatives_in_destination, ''), COALESCE(ip.trusted_person, c.trusted_person, ''),
	COALESCE(ip.education_level, ''), COALESCE(ip.specialty, ''), COALESCE(ip.trusted_person_phone, ''), COALESCE(ip.driver_license_number, ''), COALESCE(ip.education_institution_name, ''), COALESCE(ip.education_institution_address, ''), COALESCE(ip.position, ''), COALESCE(ip.visas_received, ''), COALESCE(ip.visa_refusals, ''),
	COALESCE(ip.height, c.height), COALESCE(ip.weight, c.weight), COALESCE(ip.driver_license_categories, c.driver_license_categories), COALESCE(ip.therapist_name, c.therapist_name, ''), COALESCE(ip.clinic_name, c.clinic_name, ''),
	COALESCE(ip.diseases_last3_years, c.diseases_last3_years, ''), COALESCE(ip.additional_info, c.additional_info, ''),
	COALESCE(lp.company_name, c.name, ''), COALESCE(lp.bin, c.bin_iin, ''), COALESCE(lp.legal_form, ''), COALESCE(lp.director_full_name, ''), COALESCE(lp.contact_person_name, ''),
	COALESCE(lp.contact_person_position, ''), COALESCE(lp.contact_person_phone, c.phone, ''), COALESCE(lp.contact_person_email, c.email, ''), COALESCE(lp.legal_address, c.address, ''),
	COALESCE(lp.actual_address, c.actual_address, ''), COALESCE(lp.bank_name, ''), COALESCE(lp.iban, ''), COALESCE(lp.bik, ''), COALESCE(lp.kbe, ''), COALESCE(lp.tax_regime, ''), COALESCE(lp.website, ''),
	COALESCE(lp.industry, ''), COALESCE(lp.company_size, ''), COALESCE(lp.additional_info, c.contact_info, '')
FROM clients c
LEFT JOIN client_individual_profiles ip ON ip.client_id = c.id
LEFT JOIN client_legal_profiles lp ON lp.client_id = c.id
`

const clientSelectLegacy = `
SELECT
	c.id,
	c.owner_id,
	COALESCE(NULLIF(c.client_type, ''), 'individual') AS client_type,
	COALESCE(NULLIF(c.display_name, ''), NULLIF(c.name, ''), '') AS display_name,
	COALESCE(NULLIF(c.primary_phone, ''), NULLIF(c.phone, ''), '') AS primary_phone,
	COALESCE(NULLIF(c.primary_email, ''), NULLIF(c.email, ''), '') AS primary_email,
	COALESCE(c.address, '') AS address,
	COALESCE(c.contact_info, '') AS contact_info,
	c.created_at,
	COALESCE(c.updated_at, c.created_at) AS updated_at,
	c.is_archived,
	c.archived_at,
	c.archived_by,
	COALESCE(c.archive_reason, '') AS archive_reason,
	COALESCE(c.last_name, ''), COALESCE(c.first_name, ''), COALESCE(c.middle_name, ''), COALESCE(c.iin, ''), COALESCE(c.id_number, ''), COALESCE(c.passport_series, ''), COALESCE(c.passport_number, ''),
	COALESCE(c.registration_address, ''), COALESCE(c.actual_address, ''), COALESCE(c.country, ''), COALESCE(c.trip_purpose, ''), c.birth_date, COALESCE(c.birth_place, ''),
	COALESCE(c.citizenship, ''), COALESCE(c.sex, ''), COALESCE(c.marital_status, ''), c.passport_issue_date, c.passport_expire_date,
	COALESCE(c.previous_last_name, ''), COALESCE(c.spouse_name, ''), COALESCE(c.spouse_contacts, ''), c.has_children, c.children_list,
	COALESCE(c.education, ''), COALESCE(c.job, ''), COALESCE(c.trips_last5_years, ''), COALESCE(c.relatives_in_destination, ''), COALESCE(c.trusted_person, ''),
	'' AS education_level, '' AS specialty, '' AS trusted_person_phone, '' AS driver_license_number, '' AS education_institution_name, '' AS education_institution_address, '' AS position, '' AS visas_received, '' AS visa_refusals,
	c.height, c.weight, c.driver_license_categories, COALESCE(c.therapist_name, ''), COALESCE(c.clinic_name, ''),
	COALESCE(c.diseases_last3_years, ''), COALESCE(c.additional_info, ''),
	COALESCE(c.name, ''), COALESCE(c.bin_iin, ''), '' AS legal_form, '' AS director_full_name, '' AS contact_person_name,
	'' AS contact_person_position, COALESCE(c.phone, ''), COALESCE(c.email, ''), COALESCE(c.address, ''),
	COALESCE(c.actual_address, ''), '' AS bank_name, '' AS iban, '' AS bik, '' AS kbe, '' AS tax_regime, '' AS website,
	'' AS industry, '' AS company_size, COALESCE(c.contact_info, '')
FROM clients c
`

func scanClient(scanner clientRowScanner) (*models.Client, error) {
	c := &models.Client{}
	var (
		displayName, primaryPhone, primaryEmail sql.NullString
		birthDate, passIssue, passExpire        sql.NullTime
		hasChildren                             sql.NullBool
		height, weight                          sql.NullInt64
		children, drivers                       []byte
		archivedAt                              sql.NullTime
		archivedBy                              sql.NullInt64
		archiveReason                           sql.NullString
	)
	ip := &models.ClientIndividualProfile{}
	lp := &models.ClientLegalProfile{}
	err := scanner.Scan(
		&c.ID, &c.OwnerID, &c.ClientType, &displayName, &primaryPhone, &primaryEmail, &c.Address, &c.ContactInfo, &c.CreatedAt, &c.UpdatedAt,
		&c.IsArchived, &archivedAt, &archivedBy, &archiveReason,
		&ip.LastName, &ip.FirstName, &ip.MiddleName, &ip.IIN, &ip.IDNumber, &ip.PassportSeries, &ip.PassportNumber,
		&ip.RegistrationAddress, &ip.ActualAddress, &ip.Country, &ip.TripPurpose, &birthDate, &ip.BirthPlace,
		&ip.Citizenship, &ip.Sex, &ip.MaritalStatus, &passIssue, &passExpire,
		&ip.PreviousLastName, &ip.SpouseName, &ip.SpouseContacts, &hasChildren, &children,
		&ip.Education, &ip.Job, &ip.TripsLast5Years, &ip.RelativesInDestination, &ip.TrustedPerson,
		&ip.EducationLevel, &ip.Specialty, &ip.TrustedPersonPhone, &ip.DriverLicenseNumber, &ip.EducationInstitutionName, &ip.EducationInstitutionAddress, &ip.Position, &ip.VisasReceived, &ip.VisaRefusals,
		&height, &weight, &drivers, &ip.TherapistName, &ip.ClinicName, &ip.DiseasesLast3Years, &ip.AdditionalInfo,
		&lp.CompanyName, &lp.BIN, &lp.LegalForm, &lp.DirectorFullName, &lp.ContactPersonName,
		&lp.ContactPersonPosition, &lp.ContactPersonPhone, &lp.ContactPersonEmail, &lp.LegalAddress,
		&lp.ActualAddress, &lp.BankName, &lp.IBAN, &lp.BIK, &lp.KBE, &lp.TaxRegime, &lp.Website,
		&lp.Industry, &lp.CompanySize, &lp.AdditionalInfo,
	)
	if err != nil {
		return nil, err
	}
	if displayName.Valid {
		c.DisplayName = displayName.String
	}
	if primaryPhone.Valid {
		c.PrimaryPhone = primaryPhone.String
	}
	if primaryEmail.Valid {
		c.PrimaryEmail = primaryEmail.String
	}
	c.Name, c.Phone, c.Email = c.DisplayName, c.PrimaryPhone, c.PrimaryEmail
	if archivedAt.Valid {
		v := archivedAt.Time
		c.ArchivedAt = &v
	}
	if archivedBy.Valid {
		v := int(archivedBy.Int64)
		c.ArchivedBy = &v
	}
	if archiveReason.Valid {
		c.ArchiveReason = archiveReason.String
	}
	c.BinIin = lp.BIN
	if c.ClientType == models.ClientTypeIndividual {
		if birthDate.Valid {
			t := birthDate.Time
			ip.BirthDate = &t
			c.BirthDate = &t
		}
		if passIssue.Valid {
			t := passIssue.Time
			ip.PassportIssueDate = &t
			c.PassportIssueDate = &t
		}
		if passExpire.Valid {
			t := passExpire.Time
			ip.PassportExpireDate = &t
			c.PassportExpireDate = &t
		}
		if hasChildren.Valid {
			v := hasChildren.Bool
			ip.HasChildren = &v
			c.HasChildren = &v
		}
		if height.Valid {
			v := int16(height.Int64)
			ip.Height = &v
			c.Height = &v
		}
		if weight.Valid {
			v := int16(weight.Int64)
			ip.Weight = &v
			c.Weight = &v
		}
		if len(children) > 0 {
			ip.ChildrenList = json.RawMessage(children)
			c.ChildrenList = json.RawMessage(children)
		}
		if len(drivers) > 0 {
			ip.DriverLicenseCategories = json.RawMessage(drivers)
			c.DriverLicenseCategories = json.RawMessage(drivers)
		}
		ip.ClientID = c.ID
		c.IndividualProfile = ip
		c.LastName, c.FirstName, c.MiddleName = ip.LastName, ip.FirstName, ip.MiddleName
		c.IIN, c.IDNumber, c.PassportSeries, c.PassportNumber = ip.IIN, ip.IDNumber, ip.PassportSeries, ip.PassportNumber
		c.RegistrationAddress, c.ActualAddress = ip.RegistrationAddress, ip.ActualAddress
		c.Country, c.TripPurpose = ip.Country, ip.TripPurpose
		c.BirthPlace, c.Citizenship, c.Sex, c.MaritalStatus = ip.BirthPlace, ip.Citizenship, ip.Sex, ip.MaritalStatus
		c.PreviousLastName, c.SpouseName, c.SpouseContacts = ip.PreviousLastName, ip.SpouseName, ip.SpouseContacts
		c.Education, c.EducationLevel, c.Job, c.TripsLast5Years = ip.Education, ip.EducationLevel, ip.Job, ip.TripsLast5Years
		c.RelativesInDestination, c.TrustedPerson = ip.RelativesInDestination, ip.TrustedPerson
		c.Specialty, c.TrustedPersonPhone, c.DriverLicenseNumber = ip.Specialty, ip.TrustedPersonPhone, ip.DriverLicenseNumber
		c.EducationInstitutionName, c.EducationInstitutionAddress = ip.EducationInstitutionName, ip.EducationInstitutionAddress
		c.Position, c.VisasReceived, c.VisaRefusals = ip.Position, ip.VisasReceived, ip.VisaRefusals
		c.TherapistName, c.ClinicName, c.DiseasesLast3Years, c.AdditionalInfo = ip.TherapistName, ip.ClinicName, ip.DiseasesLast3Years, ip.AdditionalInfo
	}
	if c.ClientType == models.ClientTypeLegal {
		lp.ClientID = c.ID
		c.LegalProfile = lp
		if c.BinIin == "" {
			c.BinIin = lp.BIN
		}
		if c.Address == "" {
			c.Address = lp.LegalAddress
		}
		if c.ContactInfo == "" {
			c.ContactInfo = lp.AdditionalInfo
		}
	}
	return c, nil
}

func (r *ClientRepository) Create(c *models.Client) (int64, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	now := time.Now()
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now
	}
	q := `INSERT INTO clients (owner_id, client_type, display_name, primary_phone, primary_email, address, contact_info, created_at, updated_at, name, phone, email, bin_iin)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`
	var id int64
	err = tx.QueryRow(q, c.OwnerID, c.ClientType, c.Name, nullString(c.Phone), nullString(c.Email), c.Address, c.ContactInfo, c.CreatedAt, c.UpdatedAt, c.Name, nullString(c.Phone), nullString(c.Email), nullString(c.BinIin)).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create client: %w", err)
	}
	c.ID = int(id)
	if err := upsertProfilesTx(tx, c); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func upsertProfilesTx(tx *sql.Tx, c *models.Client) error {
	if c.ClientType == models.ClientTypeIndividual {
		_, err := tx.Exec(`INSERT INTO client_individual_profiles (client_id,last_name,first_name,middle_name,iin,id_number,passport_series,passport_number,registration_address,actual_address,country,trip_purpose,birth_date,birth_place,citizenship,sex,marital_status,passport_issue_date,passport_expire_date,previous_last_name,spouse_name,spouse_contacts,has_children,children_list,education,education_level,job,trips_last5_years,relatives_in_destination,trusted_person,specialty,trusted_person_phone,driver_license_number,education_institution_name,education_institution_address,position,visas_received,visa_refusals,height,weight,driver_license_categories,therapist_name,clinic_name,diseases_last3_years,additional_info,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,$38,$39,$40,$41,$42,$43,$44,$45,NOW())
ON CONFLICT (client_id) DO UPDATE SET
last_name=EXCLUDED.last_name,first_name=EXCLUDED.first_name,middle_name=EXCLUDED.middle_name,iin=EXCLUDED.iin,id_number=EXCLUDED.id_number,passport_series=EXCLUDED.passport_series,passport_number=EXCLUDED.passport_number,registration_address=EXCLUDED.registration_address,actual_address=EXCLUDED.actual_address,country=EXCLUDED.country,trip_purpose=EXCLUDED.trip_purpose,birth_date=EXCLUDED.birth_date,birth_place=EXCLUDED.birth_place,citizenship=EXCLUDED.citizenship,sex=EXCLUDED.sex,marital_status=EXCLUDED.marital_status,passport_issue_date=EXCLUDED.passport_issue_date,passport_expire_date=EXCLUDED.passport_expire_date,previous_last_name=EXCLUDED.previous_last_name,spouse_name=EXCLUDED.spouse_name,spouse_contacts=EXCLUDED.spouse_contacts,has_children=EXCLUDED.has_children,children_list=EXCLUDED.children_list,education=EXCLUDED.education,education_level=EXCLUDED.education_level,job=EXCLUDED.job,trips_last5_years=EXCLUDED.trips_last5_years,relatives_in_destination=EXCLUDED.relatives_in_destination,trusted_person=EXCLUDED.trusted_person,specialty=EXCLUDED.specialty,trusted_person_phone=EXCLUDED.trusted_person_phone,driver_license_number=EXCLUDED.driver_license_number,education_institution_name=EXCLUDED.education_institution_name,education_institution_address=EXCLUDED.education_institution_address,position=EXCLUDED.position,visas_received=EXCLUDED.visas_received,visa_refusals=EXCLUDED.visa_refusals,height=EXCLUDED.height,weight=EXCLUDED.weight,driver_license_categories=EXCLUDED.driver_license_categories,therapist_name=EXCLUDED.therapist_name,clinic_name=EXCLUDED.clinic_name,diseases_last3_years=EXCLUDED.diseases_last3_years,additional_info=EXCLUDED.additional_info,updated_at=NOW()`,
			c.ID, c.LastName, c.FirstName, c.MiddleName, nullString(c.IIN), nullString(c.IDNumber), nullString(c.PassportSeries), nullString(c.PassportNumber),
			nullString(c.RegistrationAddress), nullString(c.ActualAddress), nullString(c.Country), nullString(c.TripPurpose), c.BirthDate, nullString(c.BirthPlace), nullString(c.Citizenship), nullString(c.Sex), nullString(c.MaritalStatus), c.PassportIssueDate, c.PassportExpireDate,
			nullString(c.PreviousLastName), nullString(c.SpouseName), nullString(c.SpouseContacts), c.HasChildren, nullRaw(c.ChildrenList), nullString(c.Education), nullString(c.EducationLevel), nullString(c.Job), nullString(c.TripsLast5Years), nullString(c.RelativesInDestination), nullString(c.TrustedPerson), nullString(c.Specialty), nullString(c.TrustedPersonPhone), nullString(c.DriverLicenseNumber), nullString(c.EducationInstitutionName), nullString(c.EducationInstitutionAddress), nullString(c.Position), nullString(c.VisasReceived), nullString(c.VisaRefusals), nullInt16(c.Height), nullInt16(c.Weight), nullRaw(c.DriverLicenseCategories), nullString(c.TherapistName), nullString(c.ClinicName), nullString(c.DiseasesLast3Years), nullString(c.AdditionalInfo))
		if err != nil {
			return fmt.Errorf("upsert individual profile: %w", err)
		}
		_, _ = tx.Exec(`DELETE FROM client_legal_profiles WHERE client_id=$1`, c.ID)
		return nil
	}
	lp := buildLegalProfileForUpsert(c)
	_, err := tx.Exec(`INSERT INTO client_legal_profiles (client_id,company_name,bin,legal_form,director_full_name,contact_person_name,contact_person_position,contact_person_phone,contact_person_email,legal_address,actual_address,bank_name,iban,bik,kbe,tax_regime,website,industry,company_size,additional_info,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,NOW())
ON CONFLICT (client_id) DO UPDATE SET
company_name=EXCLUDED.company_name,bin=EXCLUDED.bin,legal_form=EXCLUDED.legal_form,director_full_name=EXCLUDED.director_full_name,contact_person_name=EXCLUDED.contact_person_name,contact_person_position=EXCLUDED.contact_person_position,contact_person_phone=EXCLUDED.contact_person_phone,contact_person_email=EXCLUDED.contact_person_email,legal_address=EXCLUDED.legal_address,actual_address=EXCLUDED.actual_address,bank_name=EXCLUDED.bank_name,iban=EXCLUDED.iban,bik=EXCLUDED.bik,kbe=EXCLUDED.kbe,tax_regime=EXCLUDED.tax_regime,website=EXCLUDED.website,industry=EXCLUDED.industry,company_size=EXCLUDED.company_size,additional_info=EXCLUDED.additional_info,updated_at=NOW()`,
		c.ID, nullString(lp.CompanyName), nullString(lp.BIN), nullString(lp.LegalForm), nullString(lp.DirectorFullName), nullString(lp.ContactPersonName), nullString(lp.ContactPersonPosition), nullString(lp.ContactPersonPhone), nullString(lp.ContactPersonEmail), nullString(lp.LegalAddress), nullString(lp.ActualAddress), nullString(lp.BankName), nullString(lp.IBAN), nullString(lp.BIK), nullString(lp.KBE), nullString(lp.TaxRegime), nullString(lp.Website), nullString(lp.Industry), nullString(lp.CompanySize), nullString(lp.AdditionalInfo))
	if err != nil {
		return fmt.Errorf("upsert legal profile: %w", err)
	}
	_, _ = tx.Exec(`DELETE FROM client_individual_profiles WHERE client_id=$1`, c.ID)
	return nil
}

func buildLegalProfileForUpsert(c *models.Client) models.ClientLegalProfile {
	lp := models.ClientLegalProfile{
		CompanyName: c.Name,
		BIN:         c.BinIin,
	}
	if c.LegalProfile != nil {
		lp = *c.LegalProfile
		if strings.TrimSpace(lp.CompanyName) == "" {
			lp.CompanyName = c.Name
		}
		if strings.TrimSpace(lp.BIN) == "" {
			lp.BIN = c.BinIin
		}
		if strings.TrimSpace(lp.ContactPersonPhone) == "" {
			lp.ContactPersonPhone = c.Phone
		}
		if strings.TrimSpace(lp.ContactPersonEmail) == "" {
			lp.ContactPersonEmail = c.Email
		}
		if strings.TrimSpace(lp.LegalAddress) == "" {
			lp.LegalAddress = c.Address
		}
	}
	return lp
}

func (r *ClientRepository) Update(c *models.Client) error {
	q := `UPDATE clients SET owner_id=$1, client_type=$2, display_name=$3, primary_phone=$4, primary_email=$5, address=$6, contact_info=$7, updated_at=NOW(), name=$8, phone=$9, email=$10, bin_iin=$11 WHERE id=$12`
	_, err := r.db.Exec(q, c.OwnerID, c.ClientType, c.Name, nullString(c.Phone), nullString(c.Email), c.Address, c.ContactInfo, c.Name, nullString(c.Phone), nullString(c.Email), nullString(c.BinIin), c.ID)
	if err != nil {
		return fmt.Errorf("update client: %w", err)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := upsertProfilesTx(tx, c); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ClientRepository) Delete(id int) error {
	_, err := r.db.Exec(`DELETE FROM clients WHERE id=$1`, id)
	return err
}

func clientArchiveWhere(scope ArchiveScope) string {
	switch scope {
	case ArchiveScopeArchivedOnly:
		return "c.is_archived = TRUE"
	case ArchiveScopeAll:
		return "1=1"
	default:
		return "c.is_archived = FALSE"
	}
}

func (r *ClientRepository) Archive(id, archivedBy int, reason string) error {
	_, err := r.db.Exec(`
		UPDATE clients
		SET is_archived = TRUE,
		    archived_at = NOW(),
		    archived_by = $2,
		    archive_reason = $3
		WHERE id = $1
	`, id, archivedBy, reason)
	return err
}

func (r *ClientRepository) Unarchive(id int) error {
	_, err := r.db.Exec(`
		UPDATE clients
		SET is_archived = FALSE,
		    archived_at = NULL,
		    archived_by = NULL,
		    archive_reason = NULL
		WHERE id = $1
	`, id)
	return err
}

func (r *ClientRepository) GetByID(id int) (*models.Client, error) {
	return r.GetByIDWithArchiveScope(id, ArchiveScopeActiveOnly)
}

func (r *ClientRepository) GetByIDWithArchiveScope(id int, scope ArchiveScope) (*models.Client, error) {
	row := r.db.QueryRow(clientSelect+` WHERE c.id=$1 AND `+clientArchiveWhere(scope), id)
	c, err := scanClient(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *ClientRepository) GetByBIN(bin string) (*models.Client, error) {
	row := r.db.QueryRow(clientSelect+` WHERE COALESCE(lp.bin,c.bin_iin)=$1 LIMIT 1`, strings.TrimSpace(bin))
	c, err := scanClient(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}
func (r *ClientRepository) GetByIIN(iin string) (*models.Client, error) {
	row := r.db.QueryRow(clientSelect+` WHERE ip.iin=$1 LIMIT 1`, strings.TrimSpace(iin))
	c, err := scanClient(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}
func (r *ClientRepository) GetByPhone(phone string) (*models.Client, error) {
	row := r.db.QueryRow(clientSelect+` WHERE COALESCE(c.primary_phone,c.phone)=$1 LIMIT 1`, strings.TrimSpace(phone))
	c, err := scanClient(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}
func (r *ClientRepository) GetByEmail(email string) (*models.Client, error) {
	row := r.db.QueryRow(clientSelect+` WHERE LOWER(COALESCE(c.primary_email,c.email))=LOWER($1) LIMIT 1`, strings.TrimSpace(email))
	c, err := scanClient(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func (r *ClientRepository) ListAll(limit, offset int, clientType string) ([]*models.Client, error) {
	return r.ListAllWithFilterAndArchiveScope(limit, offset, ClientListFilter{ClientType: clientType}, ArchiveScopeActiveOnly)
}

func (r *ClientRepository) ListAllWithArchiveScope(limit, offset int, clientType string, scope ArchiveScope) ([]*models.Client, error) {
	return r.ListAllWithFilterAndArchiveScope(limit, offset, ClientListFilter{ClientType: clientType}, scope)
}

func (r *ClientRepository) ListAllWithFilterAndArchiveScope(limit, offset int, filter ClientListFilter, scope ArchiveScope) ([]*models.Client, error) {
	return r.listWithFilterAndArchiveScope(nil, "", limit, offset, filter, scope)
}
func (r *ClientRepository) List(limit, offset int) ([]*models.Client, error) {
	return r.ListAll(limit, offset, "")
}
func (r *ClientRepository) ListByOwner(ownerID, limit, offset int, clientType string) ([]*models.Client, error) {
	return r.ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset, ClientListFilter{ClientType: clientType}, ArchiveScopeActiveOnly)
}

func (r *ClientRepository) ListByOwnerWithArchiveScope(ownerID, limit, offset int, clientType string, scope ArchiveScope) ([]*models.Client, error) {
	return r.ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset, ClientListFilter{ClientType: clientType}, scope)
}

func (r *ClientRepository) ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset int, filter ClientListFilter, scope ArchiveScope) ([]*models.Client, error) {
	return r.listWithFilterAndArchiveScope(&ownerID, "", limit, offset, filter, scope)
}
func (r *ClientRepository) ListIndividuals(ownerID int, search string, limit, offset int) ([]*models.Client, error) {
	return r.ListIndividualsWithArchiveScope(ownerID, search, limit, offset, ArchiveScopeActiveOnly)
}

func (r *ClientRepository) ListIndividualsWithArchiveScope(ownerID int, search string, limit, offset int, scope ArchiveScope) ([]*models.Client, error) {
	filter := ClientListFilter{Query: search}
	return r.listWithFilterAndArchiveScope(nil, models.ClientTypeIndividual, limit, offset, filter, scope)
}
func (r *ClientRepository) ListCompanies(ownerID int, search string, limit, offset int) ([]*models.Client, error) {
	return r.ListCompaniesWithArchiveScope(ownerID, search, limit, offset, ArchiveScopeActiveOnly)
}

func (r *ClientRepository) ListCompaniesWithArchiveScope(ownerID int, search string, limit, offset int, scope ArchiveScope) ([]*models.Client, error) {
	filter := ClientListFilter{Query: search}
	return r.listWithFilterAndArchiveScope(nil, models.ClientTypeLegal, limit, offset, filter, scope)
}
func (r *ClientRepository) FindByName(name string) ([]*models.Client, error) {
	q := clientSelect + ` WHERE COALESCE(c.display_name,c.name) ILIKE $1 ORDER BY c.created_at DESC`
	return r.queryMany(q, like(name))
}

func (r *ClientRepository) UpdatePartial(id int, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	current, err := r.GetByID(id)
	if err != nil || current == nil {
		return err
	}
	for k, v := range updates {
		s, _ := v.(string)
		s = strings.TrimSpace(s)
		switch k {
		case "name":
			current.Name = s
		case "client_type":
			current.ClientType = strings.ToLower(s)
		case "bin_iin":
			current.BinIin = s
		case "address":
			current.Address = s
		case "contact_info":
			current.ContactInfo = s
		case "last_name":
			current.LastName = s
		case "first_name":
			current.FirstName = s
		case "middle_name":
			current.MiddleName = s
		case "iin":
			current.IIN = s
		case "id_number":
			current.IDNumber = s
		case "passport_series":
			current.PassportSeries = s
		case "passport_number":
			current.PassportNumber = s
		case "phone":
			current.Phone = s
		case "email":
			current.Email = s
		case "registration_address":
			current.RegistrationAddress = s
		case "actual_address":
			current.ActualAddress = s
		case "country":
			current.Country = s
		case "trip_purpose":
			current.TripPurpose = s
		case "birth_place":
			current.BirthPlace = s
		case "citizenship":
			current.Citizenship = s
		case "sex":
			current.Sex = s
		case "marital_status":
			current.MaritalStatus = s
		case "specialty":
			current.Specialty = s
		case "education_level":
			current.EducationLevel = s
		case "trusted_person_phone":
			current.TrustedPersonPhone = s
		case "driver_license_number":
			current.DriverLicenseNumber = s
		case "education_institution_name":
			current.EducationInstitutionName = s
		case "education_institution_address":
			current.EducationInstitutionAddress = s
		case "position":
			current.Position = s
		case "visas_received":
			current.VisasReceived = s
		case "visa_refusals":
			current.VisaRefusals = s
		case "birth_date", "passport_issue_date", "passport_expire_date":
			if t, ok := v.(*time.Time); ok {
				switch k {
				case "birth_date":
					current.BirthDate = t
				case "passport_issue_date":
					current.PassportIssueDate = t
				case "passport_expire_date":
					current.PassportExpireDate = t
				}
			}
		}
	}
	return r.Update(current)
}

func (r *ClientRepository) queryMany(q string, args ...any) ([]*models.Client, error) {
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.Client
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, fmt.Errorf("scan client row: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func like(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	return "%" + v + "%"
}

func (r *ClientRepository) listWithFilterAndArchiveScope(ownerID *int, forcedType string, limit, offset int, filter ClientListFilter, scope ArchiveScope) ([]*models.Client, error) {
	sortExpr, sortOrder := clientSortExpression(filter)
	where, args := buildClientListWhere(ownerID, forcedType, filter, scope, 1)
	args = append(args, limit, offset)
	query := clientSelect + fmt.Sprintf(` WHERE %s ORDER BY %s %s LIMIT $%d OFFSET $%d`, where, sortExpr, sortOrder, len(args)-1, len(args))
	clients, err := r.queryMany(query, args...)
	if err == nil {
		return clients, nil
	}
	primaryLabel := "list clients primary query"
	fallbackLabel := "list clients legacy fallback"
	if ownerID != nil {
		primaryLabel = "list clients by owner primary query"
		fallbackLabel = "list clients by owner legacy fallback"
	}
	err = fmt.Errorf("%s: %w", primaryLabel, err)
	if !isProfileSplitTableMissing(err) {
		return nil, err
	}
	legacyQuery := clientSelectLegacy + fmt.Sprintf(` WHERE %s ORDER BY %s %s LIMIT $%d OFFSET $%d`, where, sortExpr, sortOrder, len(args)-1, len(args))
	clients, err = r.queryMany(legacyQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fallbackLabel, err)
	}
	return clients, nil
}

func buildClientListWhere(ownerID *int, forcedType string, filter ClientListFilter, scope ArchiveScope, startAt int) (string, []any) {
	conditions := []string{clientArchiveWhere(scope)}
	args := make([]any, 0, 8)
	idx := startAt

	if ownerID != nil {
		conditions = append(conditions, fmt.Sprintf("c.owner_id = $%d", idx))
		args = append(args, *ownerID)
		idx++
	}

	clientType := strings.ToLower(strings.TrimSpace(filter.ClientType))
	if forcedType != "" {
		clientType = forcedType
	}
	if clientType == "company" {
		clientType = models.ClientTypeLegal
	}
	if clientType != "" {
		conditions = append(conditions, fmt.Sprintf("COALESCE(c.client_type, 'individual') = $%d", idx))
		args = append(args, clientType)
		idx++
	}

	if filter.Query != "" {
		conditions = append(conditions, fmt.Sprintf(`(
			LOWER(COALESCE(c.name, '')) LIKE $%d OR
			LOWER(COALESCE(c.display_name, '')) LIKE $%d OR
			LOWER(CONCAT_WS(' ', COALESCE(ip.last_name, ''), COALESCE(ip.first_name, ''), COALESCE(ip.middle_name, ''))) LIKE $%d OR
			LOWER(COALESCE(lp.company_name, '')) LIKE $%d OR
			LOWER(COALESCE(c.bin_iin, lp.bin, '')) LIKE $%d OR
			LOWER(COALESCE(ip.iin, '')) LIKE $%d OR
			LOWER(COALESCE(c.primary_phone, c.phone, lp.contact_person_phone, '')) LIKE $%d OR
			LOWER(COALESCE(c.primary_email, c.email, lp.contact_person_email, '')) LIKE $%d
		)`, idx, idx, idx, idx, idx, idx, idx, idx))
		args = append(args, "%"+strings.ToLower(filter.Query)+"%")
		idx++
	}

	if filter.HasDeals != nil {
		dealsClause := "d.client_id = c.id AND d.is_archived = FALSE"
		if statuses := clientDealStatusesFromGroup(filter.DealStatusGroup); len(statuses) > 0 {
			dealsClause += fmt.Sprintf(" AND COALESCE(d.status, 'new') = ANY($%d)", idx)
			args = append(args, pq.Array(statuses))
			idx++
		}
		existsExpr := fmt.Sprintf("EXISTS (SELECT 1 FROM deals d WHERE %s)", dealsClause)
		if *filter.HasDeals {
			conditions = append(conditions, existsExpr)
		} else {
			conditions = append(conditions, "NOT "+existsExpr)
		}
	}

	return strings.Join(conditions, " AND "), args
}

func clientDealStatusesFromGroup(group string) []string {
	switch strings.ToLower(strings.TrimSpace(group)) {
	case "active":
		return []string{"new", "in_progress", "negotiation"}
	case "completed":
		return []string{"won"}
	case "closed":
		return []string{"lost", "cancelled"}
	default:
		return nil
	}
}

func clientSortExpression(filter ClientListFilter) (string, string) {
	order := "DESC"
	if strings.EqualFold(filter.Order, "asc") {
		order = "ASC"
	}
	switch filter.SortBy {
	case "name":
		return "LOWER(COALESCE(NULLIF(c.display_name, ''), NULLIF(c.name, ''), ''))", order
	case "display_name":
		return "LOWER(COALESCE(NULLIF(c.display_name, ''), NULLIF(c.name, ''), ''))", order
	case "client_type":
		return "COALESCE(c.client_type, 'individual')", order
	default:
		return "c.created_at", order
	}
}

func isProfileSplitTableMissing(err error) bool {
	if err == nil {
		return false
	}
	if IsSQLState(err, SQLStateUndefinedTable) {
		msg := strings.ToLower(err.Error())
		return strings.Contains(msg, "client_individual_profiles") || strings.Contains(msg, "client_legal_profiles")
	}
	return false
}
func nullString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return strings.TrimSpace(v)
}
func nullRaw(v json.RawMessage) any {
	if len(v) == 0 {
		return nil
	}
	return []byte(v)
}
func nullInt16(v *int16) any {
	if v == nil {
		return nil
	}
	return int64(*v)
}
