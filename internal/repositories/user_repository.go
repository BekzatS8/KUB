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

	// verification
	VerifyUser(userID int) error
}

type userRepository struct {
	DB *sql.DB
}

func NewUserRepository(db *sql.DB) UserRepository {
	return &userRepository{DB: db}
}

func (r *userRepository) Create(user *models.User) error {
	const q = `
		INSERT INTO users (
			company_name, bin_iin, email, password_hash, role_id,
			phone, is_verified, verified_at,
			refresh_token, refresh_expires_at, refresh_revoked
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULL,NULL,FALSE)
		RETURNING id
	`
	return r.DB.QueryRow(q,
		user.CompanyName,
		user.BinIin,
		user.Email,
		user.PasswordHash,
		user.RoleID,
		user.Phone,
		user.IsVerified,
		user.VerifiedAt,
	).Scan(&user.ID)
}

func (r *userRepository) GetByID(id int) (*models.User, error) {
	const q = `
		SELECT
			id, company_name, bin_iin, email, password_hash, role_id,
			refresh_token, refresh_expires_at, refresh_revoked,
			phone, is_verified, verified_at
		FROM users
		WHERE id = $1
	`
	u := &models.User{}
	var (
		roleID     sql.NullInt64
		rt         sql.NullString
		rte        sql.NullTime
		rr         sql.NullBool
		phone      sql.NullString
		isVerified sql.NullBool
		verifiedAt sql.NullTime
	)
	err := r.DB.QueryRow(q, id).Scan(
		&u.ID, &u.CompanyName, &u.BinIin, &u.Email, &u.PasswordHash, &roleID,
		&rt, &rte, &rr,
		&phone, &isVerified, &verifiedAt,
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
	if phone.Valid {
		u.Phone = phone.String
	}
	if isVerified.Valid {
		u.IsVerified = isVerified.Bool
	}
	if verifiedAt.Valid {
		t := verifiedAt.Time
		u.VerifiedAt = &t
	}

	return u, nil
}

func (r *userRepository) Update(user *models.User) error {
	const q = `
		UPDATE users
		SET
			company_name=$1,
			bin_iin=$2,
			email=$3,
			password_hash=$4,
			role_id=$5,
			phone=$6,
			is_verified=$7,
			verified_at=$8
		WHERE id=$9
	`
	_, err := r.DB.Exec(q,
		user.CompanyName,
		user.BinIin,
		user.Email,
		user.PasswordHash,
		user.RoleID,
		user.Phone,
		user.IsVerified,
		user.VerifiedAt,
		user.ID,
	)
	return err
}

func (r *userRepository) Delete(id int) error {
	_, err := r.DB.Exec(`DELETE FROM users WHERE id=$1`, id)
	return err
}

func (r *userRepository) List(limit, offset int) ([]*models.User, error) {
	const q = `
		SELECT
			id, company_name, bin_iin, email, role_id,
			phone, is_verified, verified_at
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
		var (
			roleID     sql.NullInt64
			phone      sql.NullString
			isVerified sql.NullBool
			verifiedAt sql.NullTime
		)
		if err := rows.Scan(
			&u.ID, &u.CompanyName, &u.BinIin, &u.Email, &roleID,
			&phone, &isVerified, &verifiedAt,
		); err != nil {
			return nil, err
		}
		if roleID.Valid {
			u.RoleID = int(roleID.Int64)
		}
		if phone.Valid {
			u.Phone = phone.String
		}
		if isVerified.Valid {
			u.IsVerified = isVerified.Bool
		}
		if verifiedAt.Valid {
			t := verifiedAt.Time
			u.VerifiedAt = &t
		}
		res = append(res, u)
	}
	return res, rows.Err()
}

func (r *userRepository) GetByEmail(email string) (*models.User, error) {
	const q = `
		SELECT
			id, company_name, bin_iin, email, password_hash, role_id,
			refresh_token, refresh_expires_at, refresh_revoked,
			phone, is_verified, verified_at
		FROM users
		WHERE email = $1
	`
	u := &models.User{}
	var (
		roleID     sql.NullInt64
		rt         sql.NullString
		rte        sql.NullTime
		rr         sql.NullBool
		phone      sql.NullString
		isVerified sql.NullBool
		verifiedAt sql.NullTime
	)
	err := r.DB.QueryRow(q, email).Scan(
		&u.ID, &u.CompanyName, &u.BinIin, &u.Email, &u.PasswordHash, &roleID,
		&rt, &rte, &rr,
		&phone, &isVerified, &verifiedAt,
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
	if phone.Valid {
		u.Phone = phone.String
	}
	if isVerified.Valid {
		u.IsVerified = isVerified.Bool
	}
	if verifiedAt.Valid {
		t := verifiedAt.Time
		u.VerifiedAt = &t
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
	const q = `
		UPDATE users
		SET refresh_token=$1, refresh_expires_at=$2, refresh_revoked=FALSE
		WHERE refresh_token=$3
		RETURNING
			id, company_name, bin_iin, email, password_hash, role_id,
			phone, is_verified, verified_at
	`
	u := &models.User{}
	var (
		roleID     sql.NullInt64
		phone      sql.NullString
		isVerified sql.NullBool
		verifiedAt sql.NullTime
	)
	err := r.DB.QueryRow(q, newToken, newExpiresAt, oldToken).Scan(
		&u.ID, &u.CompanyName, &u.BinIin, &u.Email, &u.PasswordHash, &roleID,
		&phone, &isVerified, &verifiedAt,
	)
	if err != nil {
		return nil, err
	}
	if roleID.Valid {
		u.RoleID = int(roleID.Int64)
	}
	if phone.Valid {
		u.Phone = phone.String
	}
	if isVerified.Valid {
		u.IsVerified = isVerified.Bool
	}
	if verifiedAt.Valid {
		t := verifiedAt.Time
		u.VerifiedAt = &t
	}
	return u, nil
}

func (r *userRepository) ClearRefresh(userID int) error {
	_, err := r.DB.Exec(`
		UPDATE users
		SET refresh_token=NULL, refresh_expires_at=NULL, refresh_revoked=TRUE
		WHERE id=$1
	`, userID)
	return err
}

func (r *userRepository) GetByRefreshToken(token string) (*models.User, error) {
	const q = `
		SELECT
			id, company_name, bin_iin, email, password_hash, role_id,
			refresh_token, refresh_expires_at, refresh_revoked,
			phone, is_verified, verified_at
		FROM users
		WHERE refresh_token = $1
	`
	u := &models.User{}
	var (
		roleID     sql.NullInt64
		rt         sql.NullString
		rte        sql.NullTime
		rr         sql.NullBool
		phone      sql.NullString
		isVerified sql.NullBool
		verifiedAt sql.NullTime
	)
	err := r.DB.QueryRow(q, token).Scan(
		&u.ID, &u.CompanyName, &u.BinIin, &u.Email, &u.PasswordHash, &roleID,
		&rt, &rte, &rr,
		&phone, &isVerified, &verifiedAt,
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
	if phone.Valid {
		u.Phone = phone.String
	}
	if isVerified.Valid {
		u.IsVerified = isVerified.Bool
	}
	if verifiedAt.Valid {
		t := verifiedAt.Time
		u.VerifiedAt = &t
	}

	return u, nil
}

// ===== verification helpers =====

func (r *userRepository) VerifyUser(userID int) error {
	_, err := r.DB.Exec(`
		UPDATE users
		SET is_verified=TRUE, verified_at=NOW()
		WHERE id=$1
	`, userID)
	return err
}
