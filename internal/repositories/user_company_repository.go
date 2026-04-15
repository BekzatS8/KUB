package repositories

import (
	"database/sql"
	"fmt"

	"turcompany/internal/models"
)

type UserCompanyRepository interface {
	ListByUserID(userID int) ([]models.UserCompany, error)
	ReplaceUserCompanies(userID int, companyIDs []int, primaryCompanyID *int) error
	HasAccess(userID, companyID int) (bool, error)
	GetPrimaryCompanyID(userID int) (*int, error)
}

type userCompanyRepository struct {
	db *sql.DB
}

func NewUserCompanyRepository(db *sql.DB) UserCompanyRepository {
	return &userCompanyRepository{db: db}
}

func (r *userCompanyRepository) ListByUserID(userID int) ([]models.UserCompany, error) {
	const q = `
		SELECT uc.id, uc.user_id, uc.company_id, uc.is_primary, uc.is_active, uc.created_at,
		       c.id, c.name, c.legal_name, c.bin_iin, c.company_type, c.phone, c.email, c.address, c.is_active, c.created_at, c.updated_at
		FROM user_companies uc
		JOIN companies c ON c.id = uc.company_id
		WHERE uc.user_id = $1
		ORDER BY uc.is_primary DESC, uc.id ASC
	`
	rows, err := r.db.Query(q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.UserCompany, 0)
	for rows.Next() {
		var uc models.UserCompany
		var c models.Company
		var legalName, binIin, phone, email, address sql.NullString
		if err := rows.Scan(
			&uc.ID,
			&uc.UserID,
			&uc.CompanyID,
			&uc.IsPrimary,
			&uc.IsActive,
			&uc.CreatedAt,
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
		uc.Company = &c
		out = append(out, uc)
	}
	return out, rows.Err()
}

func (r *userCompanyRepository) ReplaceUserCompanies(userID int, companyIDs []int, primaryCompanyID *int) error {
	if primaryCompanyID != nil {
		found := false
		for _, id := range companyIDs {
			if id == *primaryCompanyID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("primary_company_id must be in company_ids")
		}
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM user_companies WHERE user_id = $1`, userID); err != nil {
		return err
	}

	for _, companyID := range companyIDs {
		isPrimary := primaryCompanyID != nil && companyID == *primaryCompanyID
		if _, err := tx.Exec(
			`INSERT INTO user_companies(user_id, company_id, is_primary, is_active) VALUES($1,$2,$3,TRUE)`,
			userID,
			companyID,
			isPrimary,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *userCompanyRepository) HasAccess(userID, companyID int) (bool, error) {
	const q = `
		SELECT EXISTS(
			SELECT 1
			FROM user_companies
			WHERE user_id = $1 AND company_id = $2 AND is_active = TRUE
		)
	`
	var ok bool
	if err := r.db.QueryRow(q, userID, companyID).Scan(&ok); err != nil {
		return false, err
	}
	return ok, nil
}

func (r *userCompanyRepository) GetPrimaryCompanyID(userID int) (*int, error) {
	const q = `
		SELECT company_id
		FROM user_companies
		WHERE user_id = $1 AND is_primary = TRUE AND is_active = TRUE
		ORDER BY id
		LIMIT 1
	`
	var companyID sql.NullInt64
	if err := r.db.QueryRow(q, userID).Scan(&companyID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if !companyID.Valid {
		return nil, nil
	}
	id := int(companyID.Int64)
	return &id, nil
}
