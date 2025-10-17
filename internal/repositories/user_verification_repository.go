package repositories

import (
	"database/sql"
	"fmt"
	"time"

	"turcompany/internal/models"
)

type UserVerificationRepository struct {
	DB *sql.DB
}

func NewUserVerificationRepository(db *sql.DB) *UserVerificationRepository {
	return &UserVerificationRepository{DB: db}
}

// Create — создаёт новую запись верификации (каждая отправка — новая строка).
func (r *UserVerificationRepository) Create(userID int, codeHash string, sentAt, expiresAt time.Time) (int64, error) {
	const q = `
		INSERT INTO user_verifications (user_id, code_hash, sent_at, expires_at, confirmed, attempts)
		VALUES ($1, $2, $3, $4, FALSE, 0)
		RETURNING id
	`
	var id int64
	if err := r.DB.QueryRow(q, userID, codeHash, sentAt, expiresAt).Scan(&id); err != nil {
		return 0, fmt.Errorf("user_verification create: %w", err)
	}
	return id, nil
}

// GetLatestByUserID — берём последнюю отправку (по sent_at DESC).
func (r *UserVerificationRepository) GetLatestByUserID(userID int) (*models.UserVerification, error) {
	const q = `
		SELECT id, user_id, code_hash, sent_at, expires_at, confirmed, attempts
		FROM user_verifications
		WHERE user_id = $1
		ORDER BY sent_at DESC
		LIMIT 1
	`
	row := r.DB.QueryRow(q, userID)
	var v models.UserVerification
	if err := row.Scan(&v.ID, &v.UserID, &v.CodeHash, &v.SentAt, &v.ExpiresAt, &v.Confirmed, &v.Attempts); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("user_verification latest: %w", err)
	}
	return &v, nil
}

// CountRecentSends — сколько раз отправляли за последнее окно (для троттлинга).
func (r *UserVerificationRepository) CountRecentSends(userID int, since time.Time) (int, error) {
	const q = `
		SELECT COUNT(*)
		FROM user_verifications
		WHERE user_id = $1 AND sent_at >= $2
	`
	var c int
	if err := r.DB.QueryRow(q, userID, since).Scan(&c); err != nil {
		return 0, fmt.Errorf("user_verification count recent: %w", err)
	}
	return c, nil
}

// IncrementAttempts — +1 попытка, возвращает новое значение attempts.
func (r *UserVerificationRepository) IncrementAttempts(id int64) (int, error) {
	const q = `
		UPDATE user_verifications
		SET attempts = attempts + 1
		WHERE id = $1
		RETURNING attempts
	`
	var attempts int
	if err := r.DB.QueryRow(q, id).Scan(&attempts); err != nil {
		return 0, fmt.Errorf("user_verification increment attempts: %w", err)
	}
	return attempts, nil
}

func (r *UserVerificationRepository) MarkConfirmed(id int64) error {
	_, err := r.DB.Exec(`UPDATE user_verifications SET confirmed=TRUE WHERE id=$1`, id)
	return err
}

// ExpireNow — моментально "протухаем" код (используем при превышении попыток).
func (r *UserVerificationRepository) ExpireNow(id int64) error {
	_, err := r.DB.Exec(`UPDATE user_verifications SET expires_at = NOW() WHERE id=$1`, id)
	return err
}
