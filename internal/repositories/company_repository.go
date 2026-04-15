package repositories

import (
	"database/sql"

	"github.com/lib/pq"
	"turcompany/internal/models"
)

type CompanyRepository interface {
	List() ([]models.Company, error)
	GetByID(id int) (*models.Company, error)
	CountByIDs(ids []int) (int, error)
}

type companyRepository struct {
	db *sql.DB
}

func NewCompanyRepository(db *sql.DB) CompanyRepository {
	return &companyRepository{db: db}
}

func (r *companyRepository) List() ([]models.Company, error) {
	const q = `
		SELECT id, name, legal_name, bin_iin, company_type, phone, email, address, is_active, created_at, updated_at
		FROM companies
		ORDER BY id
	`
	rows, err := r.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.Company, 0)
	for rows.Next() {
		c, err := scanCompany(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *companyRepository) GetByID(id int) (*models.Company, error) {
	const q = `
		SELECT id, name, legal_name, bin_iin, company_type, phone, email, address, is_active, created_at, updated_at
		FROM companies
		WHERE id = $1
	`
	return scanCompany(r.db.QueryRow(q, id))
}

func (r *companyRepository) CountByIDs(ids []int) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	const q = `SELECT COUNT(*) FROM companies WHERE id = ANY($1)`
	var c int
	if err := r.db.QueryRow(q, pq.Array(ids)).Scan(&c); err != nil {
		return 0, err
	}
	return c, nil
}

type companyScanner interface{ Scan(dest ...any) error }

func scanCompany(scanner companyScanner) (*models.Company, error) {
	var c models.Company
	var legalName, binIin, phone, email, address sql.NullString
	if err := scanner.Scan(
		&c.ID,
		&c.Name,
		&legalName,
		&binIin,
		&c.CompanyType,
		&phone,
		&email,
		&address,
		&c.IsActive,
		&c.CreatedAt,
		&c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if legalName.Valid {
		c.LegalName = &legalName.String
	}
	if binIin.Valid {
		c.BinIin = &binIin.String
	}
	if phone.Valid {
		c.Phone = &phone.String
	}
	if email.Valid {
		c.Email = &email.String
	}
	if address.Valid {
		c.Address = &address.String
	}
	return &c, nil
}
