package repositories

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"turcompany/internal/models"
)

type ClientRepository struct {
	db *sql.DB
}

type clientRowScanner interface {
	Scan(dest ...any) error
}

func NewClientRepository(db *sql.DB) *ClientRepository {
	return &ClientRepository{db: db}
}

func scanClient(scanner clientRowScanner) (*models.Client, error) {
	var c models.Client
	var clientType sql.NullString
	var binIin sql.NullString
	var address sql.NullString
	var contactInfo sql.NullString
	var lastName sql.NullString
	var firstName sql.NullString
	var middleName sql.NullString
	var iin sql.NullString
	var idNumber sql.NullString
	var passportSeries sql.NullString
	var passportNumber sql.NullString
	var phone sql.NullString
	var email sql.NullString
	var registrationAddress sql.NullString
	var actualAddress sql.NullString
	var country sql.NullString
	var tripPurpose sql.NullString
	var birthDate sql.NullTime
	var birthPlace sql.NullString
	var citizenship sql.NullString
	var sex sql.NullString
	var maritalStatus sql.NullString
	var passportIssueDate sql.NullTime
	var passportExpireDate sql.NullTime

	var previousLastName sql.NullString
	var spouseName sql.NullString
	var spouseContacts sql.NullString
	var hasChildren sql.NullBool
	var childrenList []byte
	var education sql.NullString
	var job sql.NullString
	var tripsLast5Years sql.NullString
	var relativesInDestination sql.NullString
	var trustedPerson sql.NullString
	var height sql.NullInt64
	var weight sql.NullInt64
	var driverLicenseCategories []byte
	var therapistName sql.NullString
	var clinicName sql.NullString
	var diseasesLast3Years sql.NullString
	var additionalInfo sql.NullString

	err := scanner.Scan(
		&c.ID,
		&c.Name,
		&clientType,
		&binIin,
		&address,
		&contactInfo,
		&lastName,
		&firstName,
		&middleName,
		&iin,
		&idNumber,
		&passportSeries,
		&passportNumber,
		&phone,
		&email,
		&registrationAddress,
		&actualAddress,
		&country,
		&tripPurpose,
		&birthDate,
		&birthPlace,
		&citizenship,
		&sex,
		&maritalStatus,
		&passportIssueDate,
		&passportExpireDate,
		&previousLastName,
		&spouseName,
		&spouseContacts,
		&hasChildren,
		&childrenList,
		&education,
		&job,
		&tripsLast5Years,
		&relativesInDestination,
		&trustedPerson,
		&height,
		&weight,
		&driverLicenseCategories,
		&therapistName,
		&clinicName,
		&diseasesLast3Years,
		&additionalInfo,
		&c.OwnerID,
		&c.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	c.ClientType = stringFromNull(clientType)
	if c.ClientType == "" {
		c.ClientType = models.ClientTypeIndividual
	}
	c.BinIin = stringFromNull(binIin)
	c.Address = stringFromNull(address)
	c.ContactInfo = stringFromNull(contactInfo)
	c.LastName = stringFromNull(lastName)
	c.FirstName = stringFromNull(firstName)
	c.MiddleName = stringFromNull(middleName)
	c.IIN = stringFromNull(iin)
	c.IDNumber = stringFromNull(idNumber)
	c.PassportSeries = stringFromNull(passportSeries)
	c.PassportNumber = stringFromNull(passportNumber)
	c.Phone = stringFromNull(phone)
	c.Email = stringFromNull(email)
	c.RegistrationAddress = stringFromNull(registrationAddress)
	c.ActualAddress = stringFromNull(actualAddress)
	c.Country = stringFromNull(country)
	c.TripPurpose = stringFromNull(tripPurpose)
	if birthDate.Valid {
		v := birthDate.Time
		c.BirthDate = &v
	}
	c.BirthPlace = stringFromNull(birthPlace)
	c.Citizenship = stringFromNull(citizenship)
	c.Sex = stringFromNull(sex)
	c.MaritalStatus = stringFromNull(maritalStatus)
	if passportIssueDate.Valid {
		v := passportIssueDate.Time
		c.PassportIssueDate = &v
	}
	if passportExpireDate.Valid {
		v := passportExpireDate.Time
		c.PassportExpireDate = &v
	}

	c.PreviousLastName = stringFromNull(previousLastName)
	c.SpouseName = stringFromNull(spouseName)
	c.SpouseContacts = stringFromNull(spouseContacts)
	if hasChildren.Valid {
		v := hasChildren.Bool
		c.HasChildren = &v
	}
	if len(childrenList) > 0 {
		c.ChildrenList = json.RawMessage(append([]byte(nil), childrenList...))
	}
	c.Education = stringFromNull(education)
	c.Job = stringFromNull(job)
	c.TripsLast5Years = stringFromNull(tripsLast5Years)
	c.RelativesInDestination = stringFromNull(relativesInDestination)
	c.TrustedPerson = stringFromNull(trustedPerson)
	if height.Valid {
		v := int16(height.Int64)
		c.Height = &v
	}
	if weight.Valid {
		v := int16(weight.Int64)
		c.Weight = &v
	}
	if len(driverLicenseCategories) > 0 {
		c.DriverLicenseCategories = json.RawMessage(append([]byte(nil), driverLicenseCategories...))
	}
	c.TherapistName = stringFromNull(therapistName)
	c.ClinicName = stringFromNull(clinicName)
	c.DiseasesLast3Years = stringFromNull(diseasesLast3Years)
	c.AdditionalInfo = stringFromNull(additionalInfo)

	return &c, nil
}

func (r *ClientRepository) Create(c *models.Client) (int64, error) {
	const q = `
        INSERT INTO clients (
                name, client_type, bin_iin, address, contact_info,
                last_name, first_name, middle_name,
                iin, id_number, passport_series, passport_number,
                phone, email, registration_address, actual_address,
                country, trip_purpose, birth_date, birth_place, citizenship, sex, marital_status, passport_issue_date, passport_expire_date,
                previous_last_name, spouse_name, spouse_contacts, has_children, children_list, education, job, trips_last5_years, relatives_in_destination, trusted_person, height, weight, driver_license_categories, therapist_name, clinic_name, diseases_last3_years, additional_info,
                owner_id, created_at
        )
        VALUES (
                $1, $2, $3, $4, $5,
                $6, $7, $8,
                $9, $10, $11, $12,
                $13, $14, $15, $16,
                $17, $18, $19, $20, $21, $22, $23, $24, $25,
                $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38, $39, $40, $41, $42,
                $43, $44
        )
        RETURNING id
	`

	var id int64
	err := r.db.QueryRow(
		q,
		c.Name,
		c.ClientType,
		nullStringFromEmpty(c.BinIin),
		c.Address,
		c.ContactInfo,
		c.LastName,
		c.FirstName,
		c.MiddleName,
		nullStringFromEmpty(c.IIN),
		c.IDNumber,
		c.PassportSeries,
		c.PassportNumber,
		c.Phone,
		c.Email,
		c.RegistrationAddress,
		c.ActualAddress,
		c.Country,
		c.TripPurpose,
		c.BirthDate,
		c.BirthPlace,
		c.Citizenship,
		c.Sex,
		c.MaritalStatus,
		c.PassportIssueDate,
		c.PassportExpireDate,
		c.PreviousLastName,
		c.SpouseName,
		c.SpouseContacts,
		c.HasChildren,
		nullRawMessage(c.ChildrenList),
		c.Education,
		c.Job,
		c.TripsLast5Years,
		c.RelativesInDestination,
		c.TrustedPerson,
		nullInt16(c.Height),
		nullInt16(c.Weight),
		nullRawMessage(c.DriverLicenseCategories),
		c.TherapistName,
		c.ClinicName,
		c.DiseasesLast3Years,
		c.AdditionalInfo,
		c.OwnerID,
		c.CreatedAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create client: %w", err)
	}
	return id, nil
}

func (r *ClientRepository) Update(c *models.Client) error {
	const q = `
        UPDATE clients
        SET
                name                = $1,
                bin_iin             = $2,
                address             = $3,
                contact_info        = $4,
                last_name           = $5,
                first_name          = $6,
                middle_name         = $7,
                iin                 = $8,
                id_number           = $9,
                passport_series     = $10,
                passport_number     = $11,
                phone               = $12,
                email               = $13,
                registration_address = $14,
                actual_address      = $15,
                country             = $16,
                trip_purpose        = $17,
                birth_date          = $18,
                birth_place         = $19,
                citizenship         = $20,
                sex                 = $21,
                marital_status      = $22,
                passport_issue_date = $23,
                passport_expire_date = $24,
                previous_last_name = $25,
                spouse_name = $26,
                spouse_contacts = $27,
                has_children = $28,
                children_list = $29,
                education = $30,
                job = $31,
                trips_last5_years = $32,
                relatives_in_destination = $33,
                trusted_person = $34,
                height = $35,
                weight = $36,
                driver_license_categories = $37,
                therapist_name = $38,
                clinic_name = $39,
                diseases_last3_years = $40,
                additional_info = $41,
                client_type         = $42,
                owner_id            = $43
        WHERE id = $44
	`

	_, err := r.db.Exec(
		q,
		c.Name,
		nullStringFromEmpty(c.BinIin),
		c.Address,
		c.ContactInfo,
		c.LastName,
		c.FirstName,
		c.MiddleName,
		nullStringFromEmpty(c.IIN),
		c.IDNumber,
		c.PassportSeries,
		c.PassportNumber,
		c.Phone,
		c.Email,
		c.RegistrationAddress,
		c.ActualAddress,
		c.Country,
		c.TripPurpose,
		c.BirthDate,
		c.BirthPlace,
		c.Citizenship,
		c.Sex,
		c.MaritalStatus,
		c.PassportIssueDate,
		c.PassportExpireDate,
		c.PreviousLastName,
		c.SpouseName,
		c.SpouseContacts,
		c.HasChildren,
		nullRawMessage(c.ChildrenList),
		c.Education,
		c.Job,
		c.TripsLast5Years,
		c.RelativesInDestination,
		c.TrustedPerson,
		nullInt16(c.Height),
		nullInt16(c.Weight),
		nullRawMessage(c.DriverLicenseCategories),
		c.TherapistName,
		c.ClinicName,
		c.DiseasesLast3Years,
		c.AdditionalInfo,
		c.ClientType,
		c.OwnerID,
		c.ID,
	)

	if err != nil {
		return fmt.Errorf("update client: %w", err)
	}
	return nil
}

func (r *ClientRepository) Delete(id int) error {
	const q = `DELETE FROM clients WHERE id = $1`
	res, err := r.db.Exec(q, id)
	if err != nil {
		return fmt.Errorf("delete client: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete client rows affected: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *ClientRepository) GetByID(id int) (*models.Client, error) {
	const q = `
        SELECT
                id,
                name,
                client_type,
                bin_iin,
                address,
                contact_info,
                last_name,
                first_name,
                middle_name,
                iin,
                id_number,
                passport_series,
                passport_number,
                phone,
                email,
                registration_address,
                actual_address,
                country,
                trip_purpose,
                birth_date,
                birth_place,
                citizenship,
                sex,
                marital_status,
                passport_issue_date,
                passport_expire_date,
                previous_last_name,
                spouse_name,
                spouse_contacts,
                has_children,
                children_list,
                education,
                job,
                trips_last5_years,
                relatives_in_destination,
                trusted_person,
                height,
                weight,
                driver_license_categories,
                therapist_name,
                clinic_name,
                diseases_last3_years,
                additional_info,
                owner_id,
                created_at
        FROM clients
        WHERE id = $1
`

	row := r.db.QueryRow(q, id)
	c, err := scanClient(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get client: %w", err)
	}
	return c, nil
}

func (r *ClientRepository) GetByBIN(bin string) (*models.Client, error) {
	const q = `
        SELECT
                id,
                name,
                client_type,
                bin_iin,
                address,
                contact_info,
                last_name,
                first_name,
                middle_name,
                iin,
                id_number,
                passport_series,
                passport_number,
                phone,
                email,
                registration_address,
                actual_address,
                country,
                trip_purpose,
                birth_date,
                birth_place,
                citizenship,
                sex,
                marital_status,
                passport_issue_date,
                passport_expire_date,
                previous_last_name,
                spouse_name,
                spouse_contacts,
                has_children,
                children_list,
                education,
                job,
                trips_last5_years,
                relatives_in_destination,
                trusted_person,
                height,
                weight,
                driver_license_categories,
                therapist_name,
                clinic_name,
                diseases_last3_years,
                additional_info,
                owner_id,
                created_at
        FROM clients
        WHERE bin_iin = $1
`

	row := r.db.QueryRow(q, bin)
	c, err := scanClient(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get client by BIN/IIN: %w", err)
	}
	return c, nil
}

func (r *ClientRepository) GetByIIN(iin string) (*models.Client, error) {
	const q = `
        SELECT
                id,
                name,
                client_type,
                bin_iin,
                address,
                contact_info,
                last_name,
                first_name,
                middle_name,
                iin,
                id_number,
                passport_series,
                passport_number,
                phone,
                email,
                registration_address,
                actual_address,
                country,
                trip_purpose,
                birth_date,
                birth_place,
                citizenship,
                sex,
                marital_status,
                passport_issue_date,
                passport_expire_date,
                previous_last_name,
                spouse_name,
                spouse_contacts,
                has_children,
                children_list,
                education,
                job,
                trips_last5_years,
                relatives_in_destination,
                trusted_person,
                height,
                weight,
                driver_license_categories,
                therapist_name,
                clinic_name,
                diseases_last3_years,
                additional_info,
                owner_id,
                created_at
        FROM clients
        WHERE iin = $1
`

	row := r.db.QueryRow(q, iin)
	c, err := scanClient(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get client by IIN: %w", err)
	}
	return c, nil
}

func (r *ClientRepository) GetByPhone(phone string) (*models.Client, error) {
	const q = `
        SELECT
                id,
                name,
                client_type,
                bin_iin,
                address,
                contact_info,
                last_name,
                first_name,
                middle_name,
                iin,
                id_number,
                passport_series,
                passport_number,
                phone,
                email,
                registration_address,
                actual_address,
                country,
                trip_purpose,
                birth_date,
                birth_place,
                citizenship,
                sex,
                marital_status,
                passport_issue_date,
                passport_expire_date,
                previous_last_name,
                spouse_name,
                spouse_contacts,
                has_children,
                children_list,
                education,
                job,
                trips_last5_years,
                relatives_in_destination,
                trusted_person,
                height,
                weight,
                driver_license_categories,
                therapist_name,
                clinic_name,
                diseases_last3_years,
                additional_info,
                owner_id,
                created_at
        FROM clients
        WHERE phone = $1
`

	row := r.db.QueryRow(q, phone)
	c, err := scanClient(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get client by phone: %w", err)
	}
	return c, nil
}

func (r *ClientRepository) ListAll(limit, offset int, clientType string) ([]*models.Client, error) {
	const q = `
        SELECT
                id,
                name,
                client_type,
                bin_iin,
                address,
                contact_info,
                last_name,
                first_name,
                middle_name,
                iin,
                id_number,
                passport_series,
                passport_number,
                phone,
                email,
                registration_address,
                actual_address,
                country,
                trip_purpose,
                birth_date,
                birth_place,
                citizenship,
                sex,
                marital_status,
                passport_issue_date,
                passport_expire_date,
                previous_last_name,
                spouse_name,
                spouse_contacts,
                has_children,
                children_list,
                education,
                job,
                trips_last5_years,
                relatives_in_destination,
                trusted_person,
                height,
                weight,
                driver_license_categories,
                therapist_name,
                clinic_name,
                diseases_last3_years,
                additional_info,
                owner_id,
                created_at
        FROM clients
        WHERE ($3 = '' OR client_type = $3)
        ORDER BY created_at DESC
        LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(q, limit, offset, strings.ToLower(strings.TrimSpace(clientType)))
	if err != nil {
		return nil, fmt.Errorf("list clients: %w", err)
	}
	defer rows.Close()

	var res []*models.Client
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, c)
	}
	return res, nil
}

func (r *ClientRepository) List(limit, offset int) ([]*models.Client, error) {
	return r.ListAll(limit, offset, "")
}

func (r *ClientRepository) ListIndividuals(ownerID int, q string, limit, offset int) ([]*models.Client, error) {
	_ = ownerID
	return r.listByTypeAndQuery(models.ClientTypeIndividual, q, limit, offset)
}

func (r *ClientRepository) ListCompanies(ownerID int, q string, limit, offset int) ([]*models.Client, error) {
	_ = ownerID
	return r.listByTypeAndQuery(models.ClientTypeLegal, q, limit, offset)
}

func (r *ClientRepository) listByTypeAndQuery(clientType, q string, limit, offset int) ([]*models.Client, error) {
	const query = `
        SELECT
                id,
                name,
                client_type,
                bin_iin,
                address,
                contact_info,
                last_name,
                first_name,
                middle_name,
                iin,
                id_number,
                passport_series,
                passport_number,
                phone,
                email,
                registration_address,
                actual_address,
                country,
                trip_purpose,
                birth_date,
                birth_place,
                citizenship,
                sex,
                marital_status,
                passport_issue_date,
                passport_expire_date,
                previous_last_name,
                spouse_name,
                spouse_contacts,
                has_children,
                children_list,
                education,
                job,
                trips_last5_years,
                relatives_in_destination,
                trusted_person,
                height,
                weight,
                driver_license_categories,
                therapist_name,
                clinic_name,
                diseases_last3_years,
                additional_info,
                owner_id,
                created_at
        FROM clients
        WHERE client_type = $1
          AND (
                $2 = ''
                OR name ILIKE $2
                OR CONCAT_WS(' ', last_name, first_name, middle_name) ILIKE $2
                OR iin ILIKE $2
                OR bin_iin ILIKE $2
                OR phone ILIKE $2
                OR email ILIKE $2
          )
        ORDER BY created_at DESC
        LIMIT $3 OFFSET $4
    `

	needle := "%" + strings.TrimSpace(q) + "%"
	if strings.TrimSpace(q) == "" {
		needle = ""
	}

	rows, err := r.db.Query(query, strings.ToLower(strings.TrimSpace(clientType)), needle, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list clients by type: %w", err)
	}
	defer rows.Close()

	var res []*models.Client
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, c)
	}
	return res, nil
}

func (r *ClientRepository) ListByOwner(ownerID, limit, offset int, clientType string) ([]*models.Client, error) {
	const q = `
        SELECT
                id,
                name,
                client_type,
                bin_iin,
                address,
                contact_info,
                last_name,
                first_name,
                middle_name,
                iin,
                id_number,
                passport_series,
                passport_number,
                phone,
                email,
                registration_address,
                actual_address,
                country,
                trip_purpose,
                birth_date,
                birth_place,
                citizenship,
                sex,
                marital_status,
                passport_issue_date,
                passport_expire_date,
                previous_last_name,
                spouse_name,
                spouse_contacts,
                has_children,
                children_list,
                education,
                job,
                trips_last5_years,
                relatives_in_destination,
                trusted_person,
                height,
                weight,
                driver_license_categories,
                therapist_name,
                clinic_name,
                diseases_last3_years,
                additional_info,
                owner_id,
                created_at
        FROM clients
        WHERE owner_id = $1
          AND ($4 = '' OR client_type = $4)
        ORDER BY created_at DESC
        LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Query(q, ownerID, limit, offset, strings.ToLower(strings.TrimSpace(clientType)))
	if err != nil {
		return nil, fmt.Errorf("list clients by owner: %w", err)
	}
	defer rows.Close()

	var res []*models.Client
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, c)
	}
	return res, nil
}

func (r *ClientRepository) FindByName(name string) ([]*models.Client, error) {
	const q = `
        SELECT
                id,
                name,
                client_type,
                bin_iin,
                address,
                contact_info,
                last_name,
                first_name,
                middle_name,
                iin,
                id_number,
                passport_series,
                passport_number,
                phone,
                email,
                registration_address,
                actual_address,
                country,
                trip_purpose,
                birth_date,
                birth_place,
                citizenship,
                sex,
                marital_status,
                passport_issue_date,
                passport_expire_date,
                previous_last_name,
                spouse_name,
                spouse_contacts,
                has_children,
                children_list,
                education,
                job,
                trips_last5_years,
                relatives_in_destination,
                trusted_person,
                height,
                weight,
                driver_license_categories,
                therapist_name,
                clinic_name,
                diseases_last3_years,
                additional_info,
                owner_id,
                created_at
        FROM clients
        WHERE LOWER(name) LIKE $1
        ORDER BY created_at DESC
`

	rows, err := r.db.Query(q, "%"+strings.ToLower(name)+"%")
	if err != nil {
		return nil, fmt.Errorf("find clients by name: %w", err)
	}
	defer rows.Close()

	var res []*models.Client
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, c)
	}
	return res, nil
}

func nullRawMessage(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return []byte(value)
}

func nullInt16(value *int16) any {
	if value == nil {
		return nil
	}
	return int64(*value)
}

func (r *ClientRepository) UpdatePartial(id int, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	allowed := map[string]bool{
		"name": true, "client_type": true, "bin_iin": true, "address": true, "contact_info": true,
		"last_name": true, "first_name": true, "middle_name": true, "iin": true, "id_number": true, "passport_series": true, "passport_number": true,
		"phone": true, "email": true, "registration_address": true, "actual_address": true, "country": true, "trip_purpose": true,
		"birth_date": true, "birth_place": true, "citizenship": true, "sex": true, "marital_status": true, "passport_issue_date": true, "passport_expire_date": true,
	}
	setParts := make([]string, 0, len(updates))
	args := make([]any, 0, len(updates)+1)
	i := 1
	for field, value := range updates {
		if !allowed[field] {
			continue
		}
		setParts = append(setParts, fmt.Sprintf("%s = $%d", field, i))
		if field == "bin_iin" || field == "iin" || field == "email" {
			if v, ok := value.(string); ok {
				args = append(args, nullStringFromEmpty(v))
			} else {
				args = append(args, value)
			}
		} else {
			args = append(args, value)
		}
		i++
	}
	if len(setParts) == 0 {
		return nil
	}
	args = append(args, id)
	q := fmt.Sprintf("UPDATE clients SET %s WHERE id = $%d", strings.Join(setParts, ", "), i)
	if _, err := r.db.Exec(q, args...); err != nil {
		return fmt.Errorf("partial update client: %w", err)
	}
	return nil
}

func (r *ClientRepository) GetByEmail(email string) (*models.Client, error) {
	const q = `
		SELECT
			id, name, client_type, bin_iin, address, contact_info, last_name, first_name, middle_name,
			iin, id_number, passport_series, passport_number, phone, email, registration_address, actual_address,
			country, trip_purpose, birth_date, birth_place, citizenship, sex, marital_status, passport_issue_date, passport_expire_date,
			previous_last_name, spouse_name, spouse_contacts, has_children, children_list, education, job, trips_last5_years,
			relatives_in_destination, trusted_person, height, weight, driver_license_categories, therapist_name, clinic_name, diseases_last3_years, additional_info,
			owner_id, created_at
		FROM clients WHERE email = $1
	`
	row := r.db.QueryRow(q, email)
	c, err := scanClient(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get client by email: %w", err)
	}
	return c, nil
}
