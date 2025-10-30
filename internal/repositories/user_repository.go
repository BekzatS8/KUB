package repositories

import (
	"context"
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

	// Telegram helpers (ЕДИНАЯ СИГНАТУРА)
	UpdateTelegramLink(userID int, chatID int64, enable bool) error
	GetByIDSimple(id int) (*models.User, error)
	GetTelegramSettings(ctx context.Context, userID int64) (chatID int64, notify bool, err error)
	GetByChatID(ctx context.Context, chatID int64) (*models.User, error)
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
			refresh_token, refresh_expires_at, refresh_revoked,
			telegram_chat_id, notify_tasks_telegram
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULL,NULL,FALSE,$9,$10)
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
		user.TelegramChatID,
		user.NotifyTasksTelegram,
	).Scan(&user.ID)
}

func (r *userRepository) GetByID(id int) (*models.User, error) {
	const q = `
		SELECT
			id, company_name, bin_iin, email, password_hash, role_id,
			refresh_token, refresh_expires_at, refresh_revoked,
			phone, is_verified, verified_at,
			COALESCE(telegram_chat_id,0), COALESCE(notify_tasks_telegram,TRUE)
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
		tgChatID   sql.NullInt64
		tgNotify   sql.NullBool
	)
	err := r.DB.QueryRow(q, id).Scan(
		&u.ID, &u.CompanyName, &u.BinIin, &u.Email, &u.PasswordHash, &roleID,
		&rt, &rte, &rr,
		&phone, &isVerified, &verifiedAt,
		&tgChatID, &tgNotify,
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
	if tgChatID.Valid {
		u.TelegramChatID = tgChatID.Int64
	}
	if tgNotify.Valid {
		u.NotifyTasksTelegram = tgNotify.Bool
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
			verified_at=$8,
			telegram_chat_id=$9,
			notify_tasks_telegram=$10
		WHERE id=$11
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
		user.TelegramChatID,
		user.NotifyTasksTelegram,
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
			phone, is_verified, verified_at,
			COALESCE(telegram_chat_id,0), COALESCE(notify_tasks_telegram,TRUE)
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
			tgChatID   sql.NullInt64
			tgNotify   sql.NullBool
		)
		if err := rows.Scan(
			&u.ID, &u.CompanyName, &u.BinIin, &u.Email, &roleID,
			&phone, &isVerified, &verifiedAt,
			&tgChatID, &tgNotify,
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
		if tgChatID.Valid {
			u.TelegramChatID = tgChatID.Int64
		}
		if tgNotify.Valid {
			u.NotifyTasksTelegram = tgNotify.Bool
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
			phone, is_verified, verified_at,
			COALESCE(telegram_chat_id,0), COALESCE(notify_tasks_telegram,TRUE)
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
		tgChatID   sql.NullInt64
		tgNotify   sql.NullBool
	)
	err := r.DB.QueryRow(q, email).Scan(
		&u.ID, &u.CompanyName, &u.BinIin, &u.Email, &u.PasswordHash, &roleID,
		&rt, &rte, &rr,
		&phone, &isVerified, &verifiedAt,
		&tgChatID, &tgNotify,
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
	if tgChatID.Valid {
		u.TelegramChatID = tgChatID.Int64
	}
	if tgNotify.Valid {
		u.NotifyTasksTelegram = tgNotify.Bool
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
			phone, is_verified, verified_at,
			COALESCE(telegram_chat_id,0), COALESCE(notify_tasks_telegram,TRUE)
	`
	u := &models.User{}
	var (
		roleID     sql.NullInt64
		phone      sql.NullString
		verifiedAt sql.NullTime
		tgChatID   sql.NullInt64
		tgNotify   sql.NullBool
	)
	err := r.DB.QueryRow(q, newToken, newExpiresAt, oldToken).Scan(
		&u.ID, &u.CompanyName, &u.BinIin, &u.Email, &u.PasswordHash, &roleID,
		&phone, &u.IsVerified, &verifiedAt,
		&tgChatID, &tgNotify,
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
	if verifiedAt.Valid {
		t := verifiedAt.Time
		u.VerifiedAt = &t
	}
	if tgChatID.Valid {
		u.TelegramChatID = tgChatID.Int64
	}
	if tgNotify.Valid {
		u.NotifyTasksTelegram = tgNotify.Bool
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
			phone, is_verified, verified_at,
			COALESCE(telegram_chat_id,0), COALESCE(notify_tasks_telegram,TRUE)
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
		tgChatID   sql.NullInt64
		tgNotify   sql.NullBool
	)
	err := r.DB.QueryRow(q, token).Scan(
		&u.ID, &u.CompanyName, &u.BinIin, &u.Email, &u.PasswordHash, &roleID,
		&rt, &rte, &rr,
		&phone, &isVerified, &verifiedAt,
		&tgChatID, &tgNotify,
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
	if tgChatID.Valid {
		u.TelegramChatID = tgChatID.Int64
	}
	if tgNotify.Valid {
		u.NotifyTasksTelegram = tgNotify.Bool
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

// ===== telegram helpers =====

func (r *userRepository) UpdateTelegramLink(userID int, chatID int64, enable bool) error {
	_, err := r.DB.Exec(`
		UPDATE users
		SET telegram_chat_id=$1, notify_tasks_telegram=$2
		WHERE id=$3
	`, chatID, enable, userID)
	return err
}

func (r *userRepository) GetByIDSimple(id int) (*models.User, error) {
	row := r.DB.QueryRow(`
		SELECT id, email, COALESCE(telegram_chat_id,0), COALESCE(notify_tasks_telegram,TRUE)
		FROM users WHERE id=$1`, id)
	var u models.User
	var tgChatID sql.NullInt64
	var tgNotify sql.NullBool
	if err := row.Scan(&u.ID, &u.Email, &tgChatID, &tgNotify); err != nil {
		return nil, err
	}
	if tgChatID.Valid {
		u.TelegramChatID = tgChatID.Int64
	}
	if tgNotify.Valid {
		u.NotifyTasksTelegram = tgNotify.Bool
	}
	return &u, nil
}

func (r *userRepository) GetTelegramSettings(ctx context.Context, userID int64) (int64, bool, error) {
	var chat sql.NullInt64
	var notify bool
	err := r.DB.QueryRowContext(ctx,
		`SELECT telegram_chat_id, notify_tasks_telegram FROM users WHERE id=$1`, userID,
	).Scan(&chat, &notify)
	if err != nil {
		return 0, false, err
	}
	if chat.Valid {
		return chat.Int64, notify, nil
	}
	return 0, notify, nil
}
func (r *userRepository) GetByChatID(ctx context.Context, chatID int64) (*models.User, error) {
	const q = `
		SELECT
			id, company_name, bin_iin, email, password_hash, role_id,
			refresh_token, refresh_expires_at, refresh_revoked,
			phone, is_verified, verified_at,
			COALESCE(telegram_chat_id,0), COALESCE(notify_tasks_telegram,TRUE)
		FROM users
		WHERE telegram_chat_id = $1
		LIMIT 1
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
		tgChatID   sql.NullInt64
		tgNotify   sql.NullBool
	)
	err := r.DB.QueryRowContext(ctx, q, chatID).Scan(
		&u.ID, &u.CompanyName, &u.BinIin, &u.Email, &u.PasswordHash, &roleID,
		&rt, &rte, &rr,
		&phone, &isVerified, &verifiedAt,
		&tgChatID, &tgNotify, // ПРИМ: тут без пробелов - это tgChatID/tgNotify как в остальных методах
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
	if tgChatID.Valid {
		u.TelegramChatID = tgChatID.Int64
	}
	if tgNotify.Valid {
		u.NotifyTasksTelegram = tgNotify.Bool
	}
	return u, nil
}
