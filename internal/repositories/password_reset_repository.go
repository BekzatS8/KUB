package repositories

import (
	"database/sql"
	"time"

	"turcompany/internal/models"
)

type PasswordResetRepository interface {
	Create(userID int, token string, expiresAt time.Time) (*models.PasswordReset, error)
	GetByToken(token string) (*models.PasswordReset, error)
	MarkUsed(id int) error
}

type passwordResetRepository struct {
	DB *sql.DB
}

func NewPasswordResetRepository(db *sql.DB) PasswordResetRepository {
	return &passwordResetRepository{DB: db}
}

func (r *passwordResetRepository) Create(userID int, token string, expiresAt time.Time) (*models.PasswordReset, error) {
	const q = `
                INSERT INTO password_resets (user_id, token, expires_at)
                VALUES ($1, $2, $3)
                RETURNING id, created_at
        `
	pr := &models.PasswordReset{UserID: userID, Token: token, ExpiresAt: expiresAt}
	if err := r.DB.QueryRow(q, userID, token, expiresAt).Scan(&pr.ID, &pr.CreatedAt); err != nil {
		return nil, err
	}
	return pr, nil
}

func (r *passwordResetRepository) GetByToken(token string) (*models.PasswordReset, error) {
	const q = `
                SELECT id, user_id, token, expires_at, used_at, created_at
                FROM password_resets
                WHERE token = $1
        `
	pr := &models.PasswordReset{}
	var usedAt sql.NullTime
	if err := r.DB.QueryRow(q, token).Scan(&pr.ID, &pr.UserID, &pr.Token, &pr.ExpiresAt, &usedAt, &pr.CreatedAt); err != nil {
		return nil, err
	}
	if usedAt.Valid {
		pr.UsedAt = &usedAt.Time
	}
	return pr, nil
}

func (r *passwordResetRepository) MarkUsed(id int) error {
	const q = `
                UPDATE password_resets SET used_at = NOW() WHERE id = $1
        `
	_, err := r.DB.Exec(q, id)
	return err
}
