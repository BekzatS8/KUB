package repositories

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"turcompany/internal/models"
)

type ClientRepository struct{ db *sql.DB }

func NewClientRepository(db *sql.DB) *ClientRepository { return &ClientRepository{db: db} }

type clientRowScanner interface{ Scan(dest ...any) error }

const clientSelect = `
SELECT
	c.id,
	c.owner_id,
	c.client_type,
	COALESCE(NULLIF(c.display_name, ''), NULLIF(c.name, '')) AS display_name,
	COALESCE(NULLIF(c.primary_phone, ''), NULLIF(c.phone, '')) AS primary_phone,
	COALESCE(NULLIF(c.primary_email, ''), NULLIF(c.email, '')) AS primary_email,
	COALESCE(c.address, '') AS address,
	COALESCE(c.contact_info, '') AS contact_info,
	c.created_at,
	COALESCE(c.updated_at, c.created_at) AS updated_at,
	COALESCE(ip.last_name, ''), COALESCE(ip.first_name, ''), COALESCE(ip.middle_name, ''), COALESCE(ip.iin, ''), COALESCE(ip.id_number, ''), COALESCE(ip.passport_series, ''), COALESCE(ip.passport_number, ''),
	COALESCE(ip.registration_address, ''), COALESCE(ip.actual_address, ''), COALESCE(ip.country, ''), COALESCE(ip.trip_purpose, ''), ip.birth_date, COALESCE(ip.birth_place, ''),
	COALESCE(ip.citizenship, ''), COALESCE(ip.sex, ''), COALESCE(ip.marital_status, ''), ip.passport_issue_date, ip.passport_expire_date,
	COALESCE(ip.previous_last_name, ''), COALESCE(ip.spouse_name, ''), COALESCE(ip.spouse_contacts, ''), ip.has_children, ip.children_list,
	COALESCE(ip.education, ''), COALESCE(ip.job, ''), COALESCE(ip.trips_last5_years, ''), COALESCE(ip.relatives_in_destination, ''), COALESCE(ip.trusted_person, ''),
	ip.height, ip.weight, ip.driver_license_categories, COALESCE(ip.therapist_name, ''), COALESCE(ip.clinic_name, ''),
	COALESCE(ip.diseases_last3_years, ''), COALESCE(ip.additional_info, ''),
	COALESCE(lp.company_name, ''), COALESCE(lp.bin, ''), COALESCE(lp.legal_form, ''), COALESCE(lp.director_full_name, ''), COALESCE(lp.contact_person_name, ''),
	COALESCE(lp.contact_person_position, ''), COALESCE(lp.contact_person_phone, ''), COALESCE(lp.contact_person_email, ''), COALESCE(lp.legal_address, ''),
	COALESCE(lp.actual_address, ''), COALESCE(lp.bank_name, ''), COALESCE(lp.iban, ''), COALESCE(lp.bik, ''), COALESCE(lp.kbe, ''), COALESCE(lp.tax_regime, ''), COALESCE(lp.website, ''),
	COALESCE(lp.industry, ''), COALESCE(lp.company_size, ''), COALESCE(lp.additional_info, '')
FROM clients c
LEFT JOIN client_individual_profiles ip ON ip.client_id = c.id
LEFT JOIN client_legal_profiles lp ON lp.client_id = c.id
`

func scanClient(scanner clientRowScanner) (*models.Client, error) {
	c := &models.Client{}
	var (
		birthDate, passIssue, passExpire sql.NullTime
		hasChildren                      sql.NullBool
		height, weight                   sql.NullInt64
		children, drivers                []byte
	)
	ip := &models.ClientIndividualProfile{}
	lp := &models.ClientLegalProfile{}
	err := scanner.Scan(
		&c.ID, &c.OwnerID, &c.ClientType, &c.DisplayName, &c.PrimaryPhone, &c.PrimaryEmail, &c.Address, &c.ContactInfo, &c.CreatedAt, &c.UpdatedAt,
		&ip.LastName, &ip.FirstName, &ip.MiddleName, &ip.IIN, &ip.IDNumber, &ip.PassportSeries, &ip.PassportNumber,
		&ip.RegistrationAddress, &ip.ActualAddress, &ip.Country, &ip.TripPurpose, &birthDate, &ip.BirthPlace,
		&ip.Citizenship, &ip.Sex, &ip.MaritalStatus, &passIssue, &passExpire,
		&ip.PreviousLastName, &ip.SpouseName, &ip.SpouseContacts, &hasChildren, &children,
		&ip.Education, &ip.Job, &ip.TripsLast5Years, &ip.RelativesInDestination, &ip.TrustedPerson,
		&height, &weight, &drivers, &ip.TherapistName, &ip.ClinicName, &ip.DiseasesLast3Years, &ip.AdditionalInfo,
		&lp.CompanyName, &lp.BIN, &lp.LegalForm, &lp.DirectorFullName, &lp.ContactPersonName,
		&lp.ContactPersonPosition, &lp.ContactPersonPhone, &lp.ContactPersonEmail, &lp.LegalAddress,
		&lp.ActualAddress, &lp.BankName, &lp.IBAN, &lp.BIK, &lp.KBE, &lp.TaxRegime, &lp.Website,
		&lp.Industry, &lp.CompanySize, &lp.AdditionalInfo,
	)
	if err != nil {
		return nil, err
	}
	c.Name, c.Phone, c.Email = c.DisplayName, c.PrimaryPhone, c.PrimaryEmail
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
		c.Education, c.Job, c.TripsLast5Years = ip.Education, ip.Job, ip.TripsLast5Years
		c.RelativesInDestination, c.TrustedPerson = ip.RelativesInDestination, ip.TrustedPerson
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
		_, err := tx.Exec(`INSERT INTO client_individual_profiles (client_id,last_name,first_name,middle_name,iin,id_number,passport_series,passport_number,registration_address,actual_address,country,trip_purpose,birth_date,birth_place,citizenship,sex,marital_status,passport_issue_date,passport_expire_date,previous_last_name,spouse_name,spouse_contacts,has_children,children_list,education,job,trips_last5_years,relatives_in_destination,trusted_person,height,weight,driver_license_categories,therapist_name,clinic_name,diseases_last3_years,additional_info,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,NOW())
ON CONFLICT (client_id) DO UPDATE SET
last_name=EXCLUDED.last_name,first_name=EXCLUDED.first_name,middle_name=EXCLUDED.middle_name,iin=EXCLUDED.iin,id_number=EXCLUDED.id_number,passport_series=EXCLUDED.passport_series,passport_number=EXCLUDED.passport_number,registration_address=EXCLUDED.registration_address,actual_address=EXCLUDED.actual_address,country=EXCLUDED.country,trip_purpose=EXCLUDED.trip_purpose,birth_date=EXCLUDED.birth_date,birth_place=EXCLUDED.birth_place,citizenship=EXCLUDED.citizenship,sex=EXCLUDED.sex,marital_status=EXCLUDED.marital_status,passport_issue_date=EXCLUDED.passport_issue_date,passport_expire_date=EXCLUDED.passport_expire_date,previous_last_name=EXCLUDED.previous_last_name,spouse_name=EXCLUDED.spouse_name,spouse_contacts=EXCLUDED.spouse_contacts,has_children=EXCLUDED.has_children,children_list=EXCLUDED.children_list,education=EXCLUDED.education,job=EXCLUDED.job,trips_last5_years=EXCLUDED.trips_last5_years,relatives_in_destination=EXCLUDED.relatives_in_destination,trusted_person=EXCLUDED.trusted_person,height=EXCLUDED.height,weight=EXCLUDED.weight,driver_license_categories=EXCLUDED.driver_license_categories,therapist_name=EXCLUDED.therapist_name,clinic_name=EXCLUDED.clinic_name,diseases_last3_years=EXCLUDED.diseases_last3_years,additional_info=EXCLUDED.additional_info,updated_at=NOW()`,
			c.ID, c.LastName, c.FirstName, c.MiddleName, nullString(c.IIN), nullString(c.IDNumber), nullString(c.PassportSeries), nullString(c.PassportNumber),
			nullString(c.RegistrationAddress), nullString(c.ActualAddress), nullString(c.Country), nullString(c.TripPurpose), c.BirthDate, nullString(c.BirthPlace), nullString(c.Citizenship), nullString(c.Sex), nullString(c.MaritalStatus), c.PassportIssueDate, c.PassportExpireDate,
			nullString(c.PreviousLastName), nullString(c.SpouseName), nullString(c.SpouseContacts), c.HasChildren, nullRaw(c.ChildrenList), nullString(c.Education), nullString(c.Job), nullString(c.TripsLast5Years), nullString(c.RelativesInDestination), nullString(c.TrustedPerson), nullInt16(c.Height), nullInt16(c.Weight), nullRaw(c.DriverLicenseCategories), nullString(c.TherapistName), nullString(c.ClinicName), nullString(c.DiseasesLast3Years), nullString(c.AdditionalInfo))
		if err != nil {
			return fmt.Errorf("upsert individual profile: %w", err)
		}
		_, _ = tx.Exec(`DELETE FROM client_legal_profiles WHERE client_id=$1`, c.ID)
		return nil
	}
	companyName := c.Name
	bin := c.BinIin
	var contactName, contactPhone, contactEmail, legalAddr, actualAddr, additional string
	if c.LegalProfile != nil {
		if strings.TrimSpace(c.LegalProfile.CompanyName) != "" {
			companyName = c.LegalProfile.CompanyName
		}
		if strings.TrimSpace(c.LegalProfile.BIN) != "" {
			bin = c.LegalProfile.BIN
		}
		contactName, contactPhone, contactEmail = c.LegalProfile.ContactPersonName, c.LegalProfile.ContactPersonPhone, c.LegalProfile.ContactPersonEmail
		legalAddr, actualAddr, additional = c.LegalProfile.LegalAddress, c.LegalProfile.ActualAddress, c.LegalProfile.AdditionalInfo
	}
	_, err := tx.Exec(`INSERT INTO client_legal_profiles (client_id,company_name,bin,contact_person_name,contact_person_phone,contact_person_email,legal_address,actual_address,additional_info,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())
ON CONFLICT (client_id) DO UPDATE SET company_name=EXCLUDED.company_name,bin=EXCLUDED.bin,contact_person_name=EXCLUDED.contact_person_name,contact_person_phone=EXCLUDED.contact_person_phone,contact_person_email=EXCLUDED.contact_person_email,legal_address=EXCLUDED.legal_address,actual_address=EXCLUDED.actual_address,additional_info=EXCLUDED.additional_info,updated_at=NOW()`,
		c.ID, nullString(companyName), nullString(bin), nullString(contactName), nullString(contactPhone), nullString(contactEmail), nullString(legalAddr), nullString(actualAddr), nullString(additional))
	if err != nil {
		return fmt.Errorf("upsert legal profile: %w", err)
	}
	_, _ = tx.Exec(`DELETE FROM client_individual_profiles WHERE client_id=$1`, c.ID)
	return nil
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

func (r *ClientRepository) GetByID(id int) (*models.Client, error) {
	row := r.db.QueryRow(clientSelect+` WHERE c.id=$1`, id)
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
	q := clientSelect + ` WHERE ($3='' OR c.client_type=$3) ORDER BY c.created_at DESC LIMIT $1 OFFSET $2`
	return r.queryMany(q, limit, offset, strings.TrimSpace(strings.ToLower(clientType)))
}
func (r *ClientRepository) List(limit, offset int) ([]*models.Client, error) {
	return r.ListAll(limit, offset, "")
}
func (r *ClientRepository) ListByOwner(ownerID, limit, offset int, clientType string) ([]*models.Client, error) {
	q := clientSelect + ` WHERE c.owner_id=$1 AND ($4='' OR c.client_type=$4) ORDER BY c.created_at DESC LIMIT $2 OFFSET $3`
	return r.queryMany(q, ownerID, limit, offset, strings.TrimSpace(strings.ToLower(clientType)))
}
func (r *ClientRepository) ListIndividuals(ownerID int, search string, limit, offset int) ([]*models.Client, error) {
	_ = ownerID
	q := clientSelect + ` WHERE c.client_type='individual' AND ($1='' OR CONCAT_WS(' ', ip.last_name, ip.first_name, ip.middle_name) ILIKE $1 OR ip.iin ILIKE $1 OR COALESCE(c.primary_phone,c.phone) ILIKE $1 OR COALESCE(c.primary_email,c.email) ILIKE $1) ORDER BY c.created_at DESC LIMIT $2 OFFSET $3`
	needle := like(search)
	return r.queryMany(q, needle, limit, offset)
}
func (r *ClientRepository) ListCompanies(ownerID int, search string, limit, offset int) ([]*models.Client, error) {
	_ = ownerID
	q := clientSelect + ` WHERE c.client_type='legal' AND ($1='' OR lp.company_name ILIKE $1 OR lp.bin ILIKE $1 OR lp.contact_person_name ILIKE $1 OR COALESCE(c.primary_phone,c.phone) ILIKE $1 OR COALESCE(c.primary_email,c.email) ILIKE $1) ORDER BY c.created_at DESC LIMIT $2 OFFSET $3`
	needle := like(search)
	return r.queryMany(q, needle, limit, offset)
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
			return nil, err
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
