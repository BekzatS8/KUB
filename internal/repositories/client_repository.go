package repositories

import (
	"database/sql"
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

	err := scanner.Scan(
		&c.ID,
		&c.Name,
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
		&c.OwnerID,
		&c.CreatedAt,
	)
	if err != nil {
		return nil, err
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

	return &c, nil
}

func (r *ClientRepository) Create(c *models.Client) (int64, error) {
	const q = `
        INSERT INTO clients (
                name, bin_iin, address, contact_info,
                last_name, first_name, middle_name,
                iin, id_number, passport_series, passport_number,
                phone, email, registration_address, actual_address,
                owner_id, created_at
        )
        VALUES (
                $1, $2, $3, $4,
                $5, $6, $7,
                $8, $9, $10, $11,
                $12, $13, $14, $15,
                $16, $17
        )
        RETURNING id
`

	var id int64
	err := r.db.QueryRow(
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
                owner_id            = $16
        WHERE id = $17
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
		c.OwnerID,
		c.ID,
	)

	if err != nil {
		return fmt.Errorf("update client: %w", err)
	}
	return nil
}

func (r *ClientRepository) GetByID(id int) (*models.Client, error) {
	const q = `
        SELECT
                id,
                name,
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

func (r *ClientRepository) ListAll(limit, offset int) ([]*models.Client, error) {
	const q = `
        SELECT
                id,
                name,
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
                owner_id,
                created_at
        FROM clients
        ORDER BY created_at DESC
        LIMIT $1 OFFSET $2
`

	rows, err := r.db.Query(q, limit, offset)
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
	return r.ListAll(limit, offset)
}

func (r *ClientRepository) ListByOwner(ownerID, limit, offset int) ([]*models.Client, error) {
	const q = `
        SELECT
                id,
                name,
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
                owner_id,
                created_at
        FROM clients
        WHERE owner_id = $1
        ORDER BY created_at DESC
        LIMIT $2 OFFSET $3
`

	rows, err := r.db.Query(q, ownerID, limit, offset)
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
