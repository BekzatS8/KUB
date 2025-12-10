package repositories

import (
	"database/sql"
	"time"

	"turcompany/internal/models"
)

type PasswordResetRepository interface {
	Create(userID int, token string, expiresAt time.Time) error
	GetByToken(token string) (*models.PasswordReset, error)
	MarkUsed(token string) error
}

type passwordResetRepository struct {
	DB *sql.DB
}

func NewPasswordResetRepository(db *sql.DB) PasswordResetRepository {
	return &passwordResetRepository{DB: db}
}

func (r *passwordResetRepository) Create(userID int, token string, expiresAt time.Time) error {
	const q = `
INSERT INTO password_resets (user_id, token, expires_at)
VALUES ($1, $2, $3)
`
	_, err := r.DB.Exec(q, userID, token, expiresAt)
	return err
}

func (r *passwordResetRepository) GetByToken(token string) (*models.PasswordReset, error) {
	const q = `
SELECT id, user_id, token, expires_at, used, created_at
FROM password_resets
WHERE token = $1
`
	pr := &models.PasswordReset{}
	if err := r.DB.QueryRow(q, token).Scan(&pr.ID, &pr.UserID, &pr.Token, &pr.ExpiresAt, &pr.Used, &pr.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return pr, nil
}

func (r *passwordResetRepository) MarkUsed(token string) error {
	const q = `
UPDATE password_resets SET used = TRUE WHERE token = $1
`
	_, err := r.DB.Exec(q, token)
	return err
}
