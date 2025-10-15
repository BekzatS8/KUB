package repositories

import (
	"database/sql"
	"time"
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

	// refresh helpers
	UpdateRefresh(userID int, token string, expiresAt time.Time) error
	RotateRefresh(oldToken, newToken string, newExpiresAt time.Time) (*models.User, error)
	ClearRefresh(userID int) error
	GetByRefreshToken(token string) (*models.User, error)
}

type userRepository struct {
	DB *sql.DB
}

func NewUserRepository(db *sql.DB) UserRepository {
	return &userRepository{DB: db}
}

func (r *userRepository) Create(user *models.User) error {
	const q = `
		INSERT INTO users (company_name, bin_iin, email, password_hash, role_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`
	return r.DB.QueryRow(q,
		user.CompanyName,
		user.BinIin,
		user.Email,
		user.PasswordHash,
		user.RoleID,
	).Scan(&user.ID)
}

func (r *userRepository) GetByID(id int) (*models.User, error) {
	const q = `
		SELECT id, company_name, bin_iin, email, password_hash, role_id,
		       refresh_token, refresh_expires_at, refresh_revoked
		FROM users WHERE id = $1
	`
	u := &models.User{}
	var (
		roleID sql.NullInt64
		rt     sql.NullString
		rte    sql.NullTime
		rr     sql.NullBool
	)
	err := r.DB.QueryRow(q, id).Scan(
		&u.ID, &u.CompanyName, &u.BinIin, &u.Email, &u.PasswordHash, &roleID,
		&rt, &rte, &rr,
	)
	if err != nil {
		return nil, err
	}
	if roleID.Valid {
		u.RoleID = int(roleID.Int64)
	}
	if rt.Valid {
		s := rt.String
		u.RefreshToken = &s
	}
	if rte.Valid {
		t := rte.Time
		u.RefreshExpiresAt = &t
	}
	if rr.Valid {
		u.RefreshRevoked = rr.Bool
	}
	return u, nil
}

func (r *userRepository) Update(user *models.User) error {
	const q = `
		UPDATE users
		SET company_name=$1, bin_iin=$2, email=$3, password_hash=$4, role_id=$5
		WHERE id=$6
	`
	_, err := r.DB.Exec(q,
		user.CompanyName, user.BinIin, user.Email, user.PasswordHash, user.RoleID, user.ID,
	)
	return err
}

func (r *userRepository) Delete(id int) error {
	_, err := r.DB.Exec(`DELETE FROM users WHERE id=$1`, id)
	return err
}

func (r *userRepository) List(limit, offset int) ([]*models.User, error) {
	const q = `
		SELECT id, company_name, bin_iin, email, role_id
		FROM users
		ORDER BY id
		LIMIT $1 OFFSET $2
	`
	rows, err := r.DB.Query(q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []*models.User
	for rows.Next() {
		u := &models.User{}
		var roleID sql.NullInt64
		if err := rows.Scan(&u.ID, &u.CompanyName, &u.BinIin, &u.Email, &roleID); err != nil {
			return nil, err
		}
		if roleID.Valid {
			u.RoleID = int(roleID.Int64)
		}
		res = append(res, u)
	}
	return res, rows.Err()
}

func (r *userRepository) GetByEmail(email string) (*models.User, error) {
	const q = `
		SELECT id, company_name, bin_iin, email, password_hash, role_id,
		       refresh_token, refresh_expires_at, refresh_revoked
		FROM users WHERE email = $1
	`
	u := &models.User{}
	var (
		roleID sql.NullInt64
		rt     sql.NullString
		rte    sql.NullTime
		rr     sql.NullBool
	)
	err := r.DB.QueryRow(q, email).Scan(
		&u.ID, &u.CompanyName, &u.BinIin, &u.Email, &u.PasswordHash, &roleID,
		&rt, &rte, &rr,
	)
	if err != nil {
		return nil, err
	}
	if roleID.Valid {
		u.RoleID = int(roleID.Int64)
	}
	if rt.Valid {
		s := rt.String
		u.RefreshToken = &s
	}
	if rte.Valid {
		t := rte.Time
		u.RefreshExpiresAt = &t
	}
	if rr.Valid {
		u.RefreshRevoked = rr.Bool
	}
	return u, nil
}

func (r *userRepository) GetCount() (int, error) {
	var c int
	err := r.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&c)
	return c, err
}

func (r *userRepository) GetCountByRole(roleID int) (int, error) {
	var c int
	err := r.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE role_id = $1`, roleID).Scan(&c)
	return c, err
}

// ===== refresh helpers =====

func (r *userRepository) UpdateRefresh(userID int, token string, expiresAt time.Time) error {
	const q = `
		UPDATE users
		SET refresh_token=$1, refresh_expires_at=$2, refresh_revoked=FALSE
		WHERE id=$3
	`
	_, err := r.DB.Exec(q, token, expiresAt, userID)
	return err
}

func (r *userRepository) RotateRefresh(oldToken, newToken string, newExpiresAt time.Time) (*models.User, error) {
	// за одну операцию: найти пользователя по старому токену и выдать новый
	const q = `
		UPDATE users
		SET refresh_token=$1, refresh_expires_at=$2, refresh_revoked=FALSE
		WHERE refresh_token=$3
		RETURNING id, company_name, bin_iin, email, password_hash, role_id
	`
	u := &models.User{}
	var roleID sql.NullInt64
	err := r.DB.QueryRow(q, newToken, newExpiresAt, oldToken).Scan(
		&u.ID, &u.CompanyName, &u.BinIin, &u.Email, &u.PasswordHash, &roleID,
	)
	if err != nil {
		return nil, err
	}
	if roleID.Valid {
		u.RoleID = int(roleID.Int64)
	}
	return u, nil
}

func (r *userRepository) ClearRefresh(userID int) error {
	_, err := r.DB.Exec(`UPDATE users SET refresh_token=NULL, refresh_expires_at=NULL, refresh_revoked=TRUE WHERE id=$1`, userID)
	return err
}

func (r *userRepository) GetByRefreshToken(token string) (*models.User, error) {
	const q = `
		SELECT id, company_name, bin_iin, email, password_hash, role_id,
		       refresh_token, refresh_expires_at, refresh_revoked
		FROM users WHERE refresh_token = $1
	`
	u := &models.User{}
	var (
		roleID sql.NullInt64
		rt     sql.NullString
		rte    sql.NullTime
		rr     sql.NullBool
	)
	err := r.DB.QueryRow(q, token).Scan(
		&u.ID, &u.CompanyName, &u.BinIin, &u.Email, &u.PasswordHash, &roleID,
		&rt, &rte, &rr,
	)
	if err != nil {
		return nil, err
	}
	if roleID.Valid {
		u.RoleID = int(roleID.Int64)
	}
	if rt.Valid {
		s := rt.String
		u.RefreshToken = &s
	}
	if rte.Valid {
		t := rte.Time
		u.RefreshExpiresAt = &t
	}
	if rr.Valid {
		u.RefreshRevoked = rr.Bool
	}
	return u, nil
}
