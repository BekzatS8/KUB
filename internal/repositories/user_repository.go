package repositories

import (
	"context"
	"database/sql"
	"strings"
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
	GetAuthByEmail(email string) (*models.User, error)
	GetCount() (int, error)
	GetCountByRole(roleID int) (int, error)
	UpdateProfile(userID int, profile *models.User) error
	UpdateAvatar(userID int, avatarURL, avatarPath, originalPath string) error
	UpdateAvatarCrop(userID int, cropX, cropY, cropScale, cropSize *float64) error
	DeleteAvatar(userID int) error
	UpdatePassword(userID int, passwordHash string) error
	UpdateRefresh(userID int, token string, expiresAt time.Time) error
	RotateRefresh(oldToken, newToken string, newExpiresAt time.Time) (*models.User, error)
	ClearRefresh(userID int) error
	GetByRefreshToken(token string) (*models.User, error)
	VerifyUser(userID int) error
	UpdateTelegramLink(userID int, chatID int64, enable bool) error
	GetByIDSimple(id int) (*models.User, error)
	GetDepartmentIDByCode(code string) (*int, error)
	GetTelegramSettings(ctx context.Context, userID int64) (chatID int64, notify bool, err error)
	GetByChatID(ctx context.Context, chatID int64) (*models.User, error)
}

type userRepository struct{ DB *sql.DB }

func NewUserRepository(db *sql.DB) UserRepository { return &userRepository{DB: db} }

func (r *userRepository) Create(user *models.User) error {
	const q = `
		INSERT INTO users (
			company_name, bin_iin, first_name, last_name, middle_name, position,
			email, password_hash, role_id, branch_id, is_active,
			phone, address, extra_info, avatar_url, avatar_path, avatar_original_path,
			avatar_crop_x, avatar_crop_y, avatar_crop_scale, avatar_crop_size,
			is_verified, verified_at,
			refresh_token, refresh_expires_at, refresh_revoked
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,FALSE,NULL,NULL,NULL,DEFAULT)
		RETURNING id
	`
	isActive := user.IsActive
	if !user.IsActiveSet && !isActive {
		isActive = true
	}
	return r.DB.QueryRow(q,
		user.CompanyName, user.BinIin,
		nullableString(user.FirstName), nullableString(user.LastName), nullableString(user.MiddleName), nullableString(user.Position),
		user.Email, user.PasswordHash, user.RoleID, user.BranchID, isActive,
		nullableString(user.Phone), nullableString(user.Address), nullableString(user.ExtraInfo),
		nullableString(user.AvatarURL), nullableString(user.AvatarPath), nullableString(user.AvatarOriginalPath),
		user.AvatarCropX, user.AvatarCropY, user.AvatarCropScale, user.AvatarCropSize,
		user.IsVerified, user.VerifiedAt,
	).Scan(&user.ID)
}

func (r *userRepository) GetByID(id int) (*models.User, error) {
	const q = `
		SELECT
			id, company_name, bin_iin, first_name, last_name, middle_name, position,
			email, password_hash, role_id, branch_id, department_id, is_active,
			refresh_token, refresh_expires_at, refresh_revoked,
			phone, address, extra_info, avatar_url, avatar_path, avatar_original_path,
			avatar_crop_x, avatar_crop_y, avatar_crop_scale, avatar_crop_size,
			is_verified, verified_at, updated_at,
			COALESCE(telegram_chat_id,0), COALESCE(notify_tasks_telegram,TRUE)
		FROM users WHERE id=$1
	`
	u, d := &models.User{}, &userDBFields{}
	if err := r.DB.QueryRow(q, id).Scan(d.dest(u)...); err != nil {
		return nil, err
	}
	d.apply(u)
	return u, nil
}

func (r *userRepository) Update(user *models.User) error {
	const q = `
		UPDATE users SET
			company_name=$1, bin_iin=$2, first_name=$3, last_name=$4, middle_name=$5, position=$6,
			email=$7, password_hash=$8, role_id=$9, branch_id=$10, is_active=$11,
			phone=$12, address=$13, extra_info=$14, avatar_url=$15, avatar_path=$16, avatar_original_path=$17,
			avatar_crop_x=$18, avatar_crop_y=$19, avatar_crop_scale=$20, avatar_crop_size=$21,
			is_verified=$22, verified_at=$23, updated_at=NOW()
		WHERE id=$24
	`
	_, err := r.DB.Exec(q,
		user.CompanyName, user.BinIin,
		nullableString(user.FirstName), nullableString(user.LastName), nullableString(user.MiddleName), nullableString(user.Position),
		user.Email, user.PasswordHash, user.RoleID, user.BranchID, user.IsActive,
		nullableString(user.Phone), nullableString(user.Address), nullableString(user.ExtraInfo),
		nullableString(user.AvatarURL), nullableString(user.AvatarPath), nullableString(user.AvatarOriginalPath),
		user.AvatarCropX, user.AvatarCropY, user.AvatarCropScale, user.AvatarCropSize,
		user.IsVerified, user.VerifiedAt,
		user.ID,
	)
	return err
}

func (r *userRepository) Delete(id int) error {
	const q = `
		UPDATE users
		SET is_active=FALSE,
			refresh_token=NULL,
			refresh_expires_at=NULL,
			refresh_revoked=TRUE,
			telegram_chat_id=NULL,
			notify_tasks_telegram=FALSE,
			email=CASE
				WHEN email LIKE 'deleted-user-%@deleted.local' THEN email
				ELSE 'deleted-user-' || id::text || '-' || EXTRACT(EPOCH FROM NOW())::bigint::text || '@deleted.local'
			END
		WHERE id=$1
	`
	_, err := r.DB.Exec(q, id)
	return err
}

func (r *userRepository) UpdatePassword(userID int, passwordHash string) error {
	_, err := r.DB.Exec(`UPDATE users SET password_hash=$1, refresh_token=NULL, refresh_expires_at=NULL, refresh_revoked=TRUE WHERE id=$2`, passwordHash, userID)
	return err
}

func (r *userRepository) List(limit, offset int) ([]*models.User, error) {
	const q = `
		SELECT
			id, company_name, bin_iin, first_name, last_name, middle_name, position,
			email, '' as password_hash, role_id, branch_id, department_id, is_active,
			NULL as refresh_token, NULL as refresh_expires_at, FALSE as refresh_revoked,
			phone, address, extra_info, avatar_url, avatar_path, avatar_original_path,
			avatar_crop_x, avatar_crop_y, avatar_crop_scale, avatar_crop_size,
			is_verified, verified_at, updated_at,
			COALESCE(telegram_chat_id,0), COALESCE(notify_tasks_telegram,TRUE)
		FROM users
		WHERE COALESCE(is_active, TRUE) = TRUE
		ORDER BY id
		LIMIT $1 OFFSET $2
	`
	rows, err := r.DB.Query(q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := make([]*models.User, 0)
	for rows.Next() {
		u, d := &models.User{}, &userDBFields{}
		if err := rows.Scan(d.dest(u)...); err != nil {
			return nil, err
		}
		d.apply(u)
		res = append(res, u)
	}
	return res, rows.Err()
}

func (r *userRepository) GetByEmail(email string) (*models.User, error) {
	const q = `
		SELECT
			id, company_name, bin_iin, first_name, last_name, middle_name, position,
			email, password_hash, role_id, branch_id, department_id, is_active,
			refresh_token, refresh_expires_at, refresh_revoked,
			phone, address, extra_info, avatar_url, avatar_path, avatar_original_path,
			avatar_crop_x, avatar_crop_y, avatar_crop_scale, avatar_crop_size,
			is_verified, verified_at, updated_at,
			COALESCE(telegram_chat_id,0), COALESCE(notify_tasks_telegram,TRUE)
		FROM users WHERE email=$1 AND COALESCE(is_active, TRUE) = TRUE
	`
	u, d := &models.User{}, &userDBFields{}
	if err := r.DB.QueryRow(q, email).Scan(d.dest(u)...); err != nil {
		return nil, err
	}
	d.apply(u)
	return u, nil
}

func (r *userRepository) GetAuthByEmail(email string) (*models.User, error) {
	const q = `
		SELECT
			id,
			COALESCE(company_name, ''),
			COALESCE(bin_iin, ''),
			COALESCE(first_name, ''),
			COALESCE(last_name, ''),
			COALESCE(middle_name, ''),
			COALESCE(position, ''),
			email,
			password_hash,
			role_id,
			branch_id,
			COALESCE(is_active, TRUE),
			refresh_token,
			refresh_expires_at,
			COALESCE(refresh_revoked, FALSE),
			COALESCE(phone, ''),
			COALESCE(is_verified, FALSE),
			verified_at,
			COALESCE(telegram_chat_id,0),
			COALESCE(notify_tasks_telegram,TRUE)
		FROM users
		WHERE email=$1
		  AND COALESCE(is_active, TRUE) = TRUE
	`
	u, d := &models.User{}, &userAuthDBFields{}
	if err := r.DB.QueryRow(q, email).Scan(d.dest(u)...); err != nil {
		return nil, err
	}
	d.apply(u)
	return u, nil
}

func (r *userRepository) GetCount() (int, error) {
	var c int
	err := r.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE COALESCE(is_active, TRUE) = TRUE`).Scan(&c)
	return c, err
}

func (r *userRepository) GetCountByRole(roleID int) (int, error) {
	var c int
	err := r.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE role_id=$1 AND COALESCE(is_active, TRUE) = TRUE`, roleID).Scan(&c)
	return c, err
}

func (r *userRepository) UpdateProfile(userID int, profile *models.User) error {
	const q = `
		UPDATE users SET
			bin_iin=$1,
			first_name=$2,
			last_name=$3,
			middle_name=$4,
			phone=$5,
			address=$6,
			extra_info=$7,
			updated_at=NOW()
		WHERE id=$8
	`
	_, err := r.DB.Exec(q,
		nullableString(profile.BinIin),
		nullableString(profile.FirstName),
		nullableString(profile.LastName),
		nullableString(profile.MiddleName),
		nullableString(profile.Phone),
		nullableString(profile.Address),
		nullableString(profile.ExtraInfo),
		userID,
	)
	return err
}

func (r *userRepository) UpdateAvatar(userID int, avatarURL, avatarPath, originalPath string) error {
	_, err := r.DB.Exec(`
		UPDATE users SET
			avatar_url=$1,
			avatar_path=$2,
			avatar_original_path=$3,
			avatar_crop_x=NULL,
			avatar_crop_y=NULL,
			avatar_crop_scale=NULL,
			avatar_crop_size=NULL,
			updated_at=NOW()
		WHERE id=$4
	`, nullableString(avatarURL), nullableString(avatarPath), nullableString(originalPath), userID)
	return err
}

func (r *userRepository) UpdateAvatarCrop(userID int, cropX, cropY, cropScale, cropSize *float64) error {
	_, err := r.DB.Exec(`
		UPDATE users SET
			avatar_crop_x=$1,
			avatar_crop_y=$2,
			avatar_crop_scale=$3,
			avatar_crop_size=$4,
			updated_at=NOW()
		WHERE id=$5
	`, cropX, cropY, cropScale, cropSize, userID)
	return err
}

func (r *userRepository) DeleteAvatar(userID int) error {
	_, err := r.DB.Exec(`
		UPDATE users SET
			avatar_url=NULL,
			avatar_path=NULL,
			avatar_original_path=NULL,
			avatar_crop_x=NULL,
			avatar_crop_y=NULL,
			avatar_crop_scale=NULL,
			avatar_crop_size=NULL,
			updated_at=NOW()
		WHERE id=$1
	`, userID)
	return err
}

func (r *userRepository) UpdateRefresh(userID int, token string, expiresAt time.Time) error {
	stored := hashRefreshToken(token)
	if stored == "" {
		stored = strings.TrimSpace(token)
	}
	_, err := r.DB.Exec(`UPDATE users SET refresh_token=$1, refresh_expires_at=$2, refresh_revoked=FALSE WHERE id=$3`, stored, expiresAt, userID)
	return err
}

func (r *userRepository) RotateRefresh(oldToken, newToken string, newExpiresAt time.Time) (*models.User, error) {
	newStored := hashRefreshToken(newToken)
	if newStored == "" {
		newStored = strings.TrimSpace(newToken)
	}
	oldRaw := strings.TrimSpace(oldToken)
	oldHashed := hashRefreshToken(oldToken)
	const q = `
		UPDATE users
		SET refresh_token=$1, refresh_expires_at=$2, refresh_revoked=FALSE
		WHERE (refresh_token=$3 OR refresh_token=$4)
		  AND COALESCE(is_active, TRUE) = TRUE
		  AND refresh_revoked = FALSE
		RETURNING
			id, company_name, bin_iin, first_name, last_name, middle_name, position,
			email, password_hash, role_id, branch_id, department_id, is_active,
			refresh_token, refresh_expires_at, refresh_revoked,
			phone, address, extra_info, avatar_url, avatar_path, avatar_original_path,
			avatar_crop_x, avatar_crop_y, avatar_crop_scale, avatar_crop_size,
			is_verified, verified_at, updated_at,
			COALESCE(telegram_chat_id,0), COALESCE(notify_tasks_telegram,TRUE)
	`
	u, d := &models.User{}, &userDBFields{}
	if err := r.DB.QueryRow(q, newStored, newExpiresAt, oldRaw, oldHashed).Scan(d.dest(u)...); err != nil {
		return nil, err
	}
	d.apply(u)
	return u, nil
}

func (r *userRepository) ClearRefresh(userID int) error {
	_, err := r.DB.Exec(`UPDATE users SET refresh_token=NULL, refresh_expires_at=NULL, refresh_revoked=TRUE WHERE id=$1`, userID)
	return err
}

func (r *userRepository) GetByRefreshToken(token string) (*models.User, error) {
	normalized := strings.TrimSpace(token)
	hashed := hashRefreshToken(token)
	const q = `
		SELECT
			id, company_name, bin_iin, first_name, last_name, middle_name, position,
			email, password_hash, role_id, branch_id, department_id, is_active,
			refresh_token, refresh_expires_at, refresh_revoked,
			phone, address, extra_info, avatar_url, avatar_path, avatar_original_path,
			avatar_crop_x, avatar_crop_y, avatar_crop_scale, avatar_crop_size,
			is_verified, verified_at, updated_at,
			COALESCE(telegram_chat_id,0), COALESCE(notify_tasks_telegram,TRUE)
		FROM users
		WHERE (refresh_token=$1 OR refresh_token=$2)
		  AND COALESCE(is_active, TRUE) = TRUE
	`
	u, d := &models.User{}, &userDBFields{}
	if err := r.DB.QueryRow(q, normalized, hashed).Scan(d.dest(u)...); err != nil {
		return nil, err
	}
	d.apply(u)
	if u.RefreshToken != nil && isHashedRefreshToken(*u.RefreshToken) {
		u.RefreshToken = nil
	}
	return u, nil
}

func (r *userRepository) VerifyUser(userID int) error {
	_, err := r.DB.Exec(`UPDATE users SET is_verified=TRUE, verified_at=NOW() WHERE id=$1`, userID)
	return err
}

func (r *userRepository) UpdateTelegramLink(userID int, chatID int64, enable bool) error {
	if chatID == 0 {
		_, err := r.DB.Exec(`UPDATE users SET telegram_chat_id=NULL, notify_tasks_telegram=FALSE WHERE id=$1`, userID)
		return err
	}
	tx, err := r.DB.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE users SET telegram_chat_id=NULL, notify_tasks_telegram=FALSE WHERE telegram_chat_id=$1 AND id<>$2`, chatID, userID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err = tx.Exec(`UPDATE users SET telegram_chat_id=$1, notify_tasks_telegram=$2 WHERE id=$3`, chatID, enable, userID); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (r *userRepository) GetByIDSimple(id int) (*models.User, error) {
	row := r.DB.QueryRow(`SELECT id, email, COALESCE(telegram_chat_id,0), COALESCE(notify_tasks_telegram,TRUE) FROM users WHERE id=$1`, id)
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
	err := r.DB.QueryRowContext(ctx, `
		SELECT telegram_chat_id,
		       CASE WHEN COALESCE(is_active, TRUE) THEN notify_tasks_telegram ELSE FALSE END
		FROM users
		WHERE id=$1
	`, userID).Scan(&chat, &notify)
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
			id, company_name, bin_iin, first_name, last_name, middle_name, position,
			email, password_hash, role_id, branch_id, department_id, is_active,
			refresh_token, refresh_expires_at, refresh_revoked,
			phone, address, extra_info, avatar_url, avatar_path, avatar_original_path,
			avatar_crop_x, avatar_crop_y, avatar_crop_scale, avatar_crop_size,
			is_verified, verified_at, updated_at,
			COALESCE(telegram_chat_id,0), COALESCE(notify_tasks_telegram,TRUE)
		FROM users
		WHERE telegram_chat_id=$1
		  AND COALESCE(is_active, TRUE) = TRUE
		LIMIT 1
	`
	u, d := &models.User{}, &userDBFields{}
	if err := r.DB.QueryRowContext(ctx, q, chatID).Scan(d.dest(u)...); err != nil {
		return nil, err
	}
	d.apply(u)
	return u, nil
}

func (r *userRepository) GetDepartmentIDByCode(code string) (*int, error) {
	var id int
	err := r.DB.QueryRow(`SELECT id FROM departments WHERE code = $1 AND is_active = TRUE LIMIT 1`, code).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

type userDBFields struct {
	companyName, binIin                       sql.NullString
	firstName, lastName, middleName, position sql.NullString
	address, extraInfo                        sql.NullString
	avatarURL, avatarPath, avatarOriginalPath sql.NullString
	roleID, branchID, departmentID            sql.NullInt64
	isActive, rr, isVerified, tgNotify        sql.NullBool
	rt, phone                                 sql.NullString
	avatarCropX, avatarCropY                  sql.NullFloat64
	avatarCropScale, avatarCropSize           sql.NullFloat64
	rte, verifiedAt, updatedAt                sql.NullTime
	tgChatID                                  sql.NullInt64
}

func (d *userDBFields) dest(u *models.User) []interface{} {
	return []interface{}{
		&u.ID, &d.companyName, &d.binIin, &d.firstName, &d.lastName, &d.middleName, &d.position,
		&u.Email, &u.PasswordHash, &d.roleID, &d.branchID, &d.departmentID, &d.isActive,
		&d.rt, &d.rte, &d.rr,
		&d.phone, &d.address, &d.extraInfo, &d.avatarURL, &d.avatarPath, &d.avatarOriginalPath,
		&d.avatarCropX, &d.avatarCropY, &d.avatarCropScale, &d.avatarCropSize,
		&d.isVerified, &d.verifiedAt, &d.updatedAt,
		&d.tgChatID, &d.tgNotify,
	}
}

func (d *userDBFields) apply(u *models.User) {
	if d.companyName.Valid {
		u.CompanyName = d.companyName.String
	}
	if d.binIin.Valid {
		u.BinIin = d.binIin.String
	}
	if d.firstName.Valid {
		u.FirstName = d.firstName.String
	}
	if d.lastName.Valid {
		u.LastName = d.lastName.String
	}
	if d.middleName.Valid {
		u.MiddleName = d.middleName.String
	}
	if d.position.Valid {
		u.Position = d.position.String
	}
	if d.roleID.Valid {
		u.RoleID = int(d.roleID.Int64)
	}
	if d.branchID.Valid {
		bid := int(d.branchID.Int64)
		u.BranchID = &bid
	}
	if d.departmentID.Valid {
		did := int(d.departmentID.Int64)
		u.DepartmentID = &did
	}
	if d.isActive.Valid {
		u.IsActive = d.isActive.Bool
	}
	if d.rt.Valid {
		v := d.rt.String
		u.RefreshToken = &v
	}
	if d.rte.Valid {
		v := d.rte.Time
		u.RefreshExpiresAt = &v
	}
	if d.rr.Valid {
		u.RefreshRevoked = d.rr.Bool
	}
	if d.phone.Valid {
		u.Phone = d.phone.String
	}
	if d.address.Valid {
		u.Address = d.address.String
	}
	if d.extraInfo.Valid {
		u.ExtraInfo = d.extraInfo.String
	}
	if d.avatarURL.Valid {
		u.AvatarURL = d.avatarURL.String
	}
	if d.avatarPath.Valid {
		u.AvatarPath = d.avatarPath.String
	}
	if d.avatarOriginalPath.Valid {
		u.AvatarOriginalPath = d.avatarOriginalPath.String
	}
	if d.avatarCropX.Valid {
		v := d.avatarCropX.Float64
		u.AvatarCropX = &v
	}
	if d.avatarCropY.Valid {
		v := d.avatarCropY.Float64
		u.AvatarCropY = &v
	}
	if d.avatarCropScale.Valid {
		v := d.avatarCropScale.Float64
		u.AvatarCropScale = &v
	}
	if d.avatarCropSize.Valid {
		v := d.avatarCropSize.Float64
		u.AvatarCropSize = &v
	}
	if d.isVerified.Valid {
		u.IsVerified = d.isVerified.Bool
	}
	if d.verifiedAt.Valid {
		v := d.verifiedAt.Time
		u.VerifiedAt = &v
	}
	if d.updatedAt.Valid {
		v := d.updatedAt.Time
		u.UpdatedAt = &v
	}
	if d.tgChatID.Valid {
		u.TelegramChatID = d.tgChatID.Int64
	}
	u.IIN = u.BinIin
	if d.tgNotify.Valid {
		u.NotifyTasksTelegram = d.tgNotify.Bool
	}
}

type userAuthDBFields struct {
	companyName, binIin                       sql.NullString
	firstName, lastName, middleName, position sql.NullString
	roleID, branchID                          sql.NullInt64
	isActive, rr, isVerified, tgNotify        sql.NullBool
	rt, phone                                 sql.NullString
	rte, verifiedAt                           sql.NullTime
	tgChatID                                  sql.NullInt64
}

func (d *userAuthDBFields) dest(u *models.User) []interface{} {
	return []interface{}{
		&u.ID, &d.companyName, &d.binIin, &d.firstName, &d.lastName, &d.middleName, &d.position,
		&u.Email, &u.PasswordHash, &d.roleID, &d.branchID, &d.isActive,
		&d.rt, &d.rte, &d.rr,
		&d.phone, &d.isVerified, &d.verifiedAt,
		&d.tgChatID, &d.tgNotify,
	}
}

func (d *userAuthDBFields) apply(u *models.User) {
	if d.companyName.Valid {
		u.CompanyName = d.companyName.String
	}
	if d.binIin.Valid {
		u.BinIin = d.binIin.String
	}
	if d.firstName.Valid {
		u.FirstName = d.firstName.String
	}
	if d.lastName.Valid {
		u.LastName = d.lastName.String
	}
	if d.middleName.Valid {
		u.MiddleName = d.middleName.String
	}
	if d.position.Valid {
		u.Position = d.position.String
	}
	if d.roleID.Valid {
		u.RoleID = int(d.roleID.Int64)
	}
	if d.branchID.Valid {
		bid := int(d.branchID.Int64)
		u.BranchID = &bid
	}
	if d.isActive.Valid {
		u.IsActive = d.isActive.Bool
	}
	if d.rt.Valid {
		v := d.rt.String
		u.RefreshToken = &v
	}
	if d.rte.Valid {
		v := d.rte.Time
		u.RefreshExpiresAt = &v
	}
	if d.rr.Valid {
		u.RefreshRevoked = d.rr.Bool
	}
	if d.phone.Valid {
		u.Phone = d.phone.String
	}
	if d.isVerified.Valid {
		u.IsVerified = d.isVerified.Bool
	}
	if d.verifiedAt.Valid {
		v := d.verifiedAt.Time
		u.VerifiedAt = &v
	}
	if d.tgChatID.Valid {
		u.TelegramChatID = d.tgChatID.Int64
	}
	u.IIN = u.BinIin
	if d.tgNotify.Valid {
		u.NotifyTasksTelegram = d.tgNotify.Bool
	}
}

func nullableString(v string) interface{} {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return strings.TrimSpace(v)
}
