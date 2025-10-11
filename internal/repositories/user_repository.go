package repositories

import (
	"database/sql"

	"turcompany/internal/models"
)

type UserRepository interface {
	Create(user *models.User) error
	GetByID(id int) (*models.User, error)
	Update(user *models.User) error
	Delete(id int) error
	List(limit, offset int) ([]*models.User, error)
	GetByEmail(email string) (*models.User, error)
	GetCount() (int, error)
	GetCountByRole(roleID int) (int, error)
}

type userRepository struct {
	DB *sql.DB
}

func NewUserRepository(db *sql.DB) UserRepository {
	return &userRepository{DB: db}
}

func (r *userRepository) Create(user *models.User) error {
	query := `
		INSERT INTO users (company_name, bin_iin, email, password_hash, role_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	return r.DB.QueryRow(query,
		user.CompanyName,
		user.BinIin,
		user.Email,
		user.PasswordHash,
		user.RoleID,
	).Scan(&user.ID)
}

func (r *userRepository) GetByID(id int) (*models.User, error) {
	query := `
		SELECT id, company_name, bin_iin, email, role_id
		FROM users
		WHERE id = $1
	`
	user := &models.User{}
	err := r.DB.QueryRow(query, id).Scan(
		&user.ID,
		&user.CompanyName,
		&user.BinIin,
		&user.Email,
		&user.RoleID,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *userRepository) Update(user *models.User) error {
	query := `
		UPDATE users
		SET company_name = $1, bin_iin = $2, email = $3, password_hash = $4, role_id = $5
		WHERE id = $6
	`
	_, err := r.DB.Exec(query,
		user.CompanyName,
		user.BinIin,
		user.Email,
		user.PasswordHash,
		user.RoleID,
		user.ID,
	)
	return err
}

func (r *userRepository) Delete(id int) error {
	query := `DELETE FROM users WHERE id = $1`
	_, err := r.DB.Exec(query, id)
	return err
}

func (r *userRepository) List(limit, offset int) ([]*models.User, error) {
	query := `
		SELECT id, company_name, bin_iin, email, role_id
		FROM users
		ORDER BY id
		LIMIT $1 OFFSET $2
	`
	rows, err := r.DB.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		u := &models.User{}
		if err := rows.Scan(
			&u.ID,
			&u.CompanyName,
			&u.BinIin,
			&u.Email,
			&u.RoleID,
		); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (r *userRepository) GetByEmail(email string) (*models.User, error) {
	query := `
		SELECT id, company_name, bin_iin, email, password_hash, role_id
		FROM users
		WHERE email = $1
	`
	user := &models.User{}
	err := r.DB.QueryRow(query, email).Scan(
		&user.ID,
		&user.CompanyName,
		&user.BinIin,
		&user.Email,
		&user.PasswordHash,
		&user.RoleID,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *userRepository) GetCount() (int, error) {
	query := `
		SELECT COUNT(*) FROM users
	`
	var count int
	err := r.DB.QueryRow(query).Scan(&count)
	return count, err
}

func (r *userRepository) GetCountByRole(roleID int) (int, error) {
	query := `
		SELECT COUNT(*) FROM users WHERE role_id = $1
	`
	var count int
	err := r.DB.QueryRow(query, roleID).Scan(&count)
	return count, err
}
