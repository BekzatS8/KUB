package repositories

import "database/sql"

type UserActiveCompanyRepository interface {
	SetActiveCompanyID(userID int, companyID *int) error
	GetActiveCompanyID(userID int) (*int, error)
}

type userActiveCompanyRepository struct {
	db *sql.DB
}

func NewUserActiveCompanyRepository(db *sql.DB) UserActiveCompanyRepository {
	return &userActiveCompanyRepository{db: db}
}

func (r *userActiveCompanyRepository) SetActiveCompanyID(userID int, companyID *int) error {
	_, err := r.db.Exec(`UPDATE users SET active_company_id = $1 WHERE id = $2`, companyID, userID)
	return err
}

func (r *userActiveCompanyRepository) GetActiveCompanyID(userID int) (*int, error) {
	var v sql.NullInt64
	err := r.db.QueryRow(`SELECT active_company_id FROM users WHERE id = $1`, userID).Scan(&v)
	if err != nil {
		return nil, err
	}
	if !v.Valid {
		return nil, nil
	}
	id := int(v.Int64)
	return &id, nil
}
