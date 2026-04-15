package repositories

import (
	"database/sql"
	"turcompany/internal/models"
)

type BranchRepository interface {
	Create(branch *models.Branch) error
	GetByID(id int) (*models.Branch, error)
	List() ([]*models.Branch, error)
	Update(branch *models.Branch) error
	Delete(id int) error
}

type branchRepository struct{ db *sql.DB }

func NewBranchRepository(db *sql.DB) BranchRepository { return &branchRepository{db: db} }

func (r *branchRepository) Create(branch *models.Branch) error {
	return r.db.QueryRow(`
		INSERT INTO branches (name, code, address, phone, email, is_active)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id, created_at, updated_at
	`, branch.Name, branch.Code, nullableString(branch.Address), nullableString(branch.Phone), nullableString(branch.Email), branch.IsActive).
		Scan(&branch.ID, &branch.CreatedAt, &branch.UpdatedAt)
}

func (r *branchRepository) GetByID(id int) (*models.Branch, error) {
	b := &models.Branch{}
	var address, phone, email sql.NullString
	err := r.db.QueryRow(`
		SELECT id, name, code, address, phone, email, is_active, created_at, updated_at
		FROM branches WHERE id=$1
	`, id).Scan(&b.ID, &b.Name, &b.Code, &address, &phone, &email, &b.IsActive, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if address.Valid {
		b.Address = address.String
	}
	if phone.Valid {
		b.Phone = phone.String
	}
	if email.Valid {
		b.Email = email.String
	}
	return b, nil
}

func (r *branchRepository) List() ([]*models.Branch, error) {
	rows, err := r.db.Query(`
		SELECT id, name, code, address, phone, email, is_active, created_at, updated_at
		FROM branches
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*models.Branch, 0)
	for rows.Next() {
		b := &models.Branch{}
		var address, phone, email sql.NullString
		if err := rows.Scan(&b.ID, &b.Name, &b.Code, &address, &phone, &email, &b.IsActive, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		if address.Valid {
			b.Address = address.String
		}
		if phone.Valid {
			b.Phone = phone.String
		}
		if email.Valid {
			b.Email = email.String
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *branchRepository) Update(branch *models.Branch) error {
	_, err := r.db.Exec(`
		UPDATE branches
		SET name=$1, code=$2, address=$3, phone=$4, email=$5, is_active=$6, updated_at=NOW()
		WHERE id=$7
	`, branch.Name, branch.Code, nullableString(branch.Address), nullableString(branch.Phone), nullableString(branch.Email), branch.IsActive, branch.ID)
	return err
}

func (r *branchRepository) Delete(id int) error {
	_, err := r.db.Exec(`DELETE FROM branches WHERE id=$1`, id)
	return err
}
