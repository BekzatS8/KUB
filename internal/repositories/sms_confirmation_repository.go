package repositories

import (
	"database/sql"
	"fmt"
	"turcompany/internal/models"
)

type SMSConfirmationRepository struct {
	DB *sql.DB
}

func NewSMSConfirmationRepository(db *sql.DB) *SMSConfirmationRepository {
	return &SMSConfirmationRepository{DB: db}
}

func (r *SMSConfirmationRepository) Create(sms *models.SMSConfirmation) (int64, error) {
	query := `INSERT INTO sms_confirmations (document_id, phone, sms_code, sent_at, confirmed, confirmed_at)
              VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	err := r.DB.QueryRow(query, sms.DocumentID, sms.Phone, sms.SMSCode, sms.SentAt, sms.Confirmed, sms.ConfirmedAt).Scan(&sms.ID)
	if err != nil {
		return 0, fmt.Errorf("create sms confirmation: %w", err)
	}
	return sms.ID, nil
}

func (r *SMSConfirmationRepository) GetByID(id int64) (*models.SMSConfirmation, error) {
	query := `SELECT id, document_id, phone, sms_code, sent_at, confirmed, confirmed_at
              FROM sms_confirmations WHERE id = $1`
	row := r.DB.QueryRow(query, id)

	var sms models.SMSConfirmation
	err := row.Scan(&sms.ID, &sms.DocumentID, &sms.Phone, &sms.SMSCode, &sms.SentAt, &sms.Confirmed, &sms.ConfirmedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get sms confirmation: %w", err)
	}

	return &sms, nil
}

func (r *SMSConfirmationRepository) GetLatestByDocumentID(documentID int64) (*models.SMSConfirmation, error) {
	query := `SELECT id, document_id, phone, sms_code, sent_at, confirmed, confirmed_at
              FROM sms_confirmations
              WHERE document_id = $1
              ORDER BY sent_at DESC LIMIT 1`
	row := r.DB.QueryRow(query, documentID)

	var sms models.SMSConfirmation
	err := row.Scan(&sms.ID, &sms.DocumentID, &sms.Phone, &sms.SMSCode, &sms.SentAt, &sms.Confirmed, &sms.ConfirmedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get latest sms confirmation: %w", err)
	}

	return &sms, nil
}

func (r *SMSConfirmationRepository) Update(sms *models.SMSConfirmation) error {
	query := `UPDATE sms_confirmations
              SET document_id = $1, phone = $2, sms_code = $3, sent_at = $4, confirmed = $5, confirmed_at = $6
              WHERE id = $7`
	_, err := r.DB.Exec(query, sms.DocumentID, sms.Phone, sms.SMSCode, sms.SentAt, sms.Confirmed, sms.ConfirmedAt, sms.ID)
	if err != nil {
		return fmt.Errorf("update sms confirmation: %w", err)
	}
	return nil
}

func (r *SMSConfirmationRepository) Delete(id int64) error {
	query := `DELETE FROM sms_confirmations WHERE id = $1`
	_, err := r.DB.Exec(query, id)
	if err != nil {
		return fmt.Errorf("delete sms confirmation: %w", err)
	}
	return nil
}

func (r *SMSConfirmationRepository) GetByDocumentIDAndCode(documentID int64, code string) (*models.SMSConfirmation, error) {
	query := `SELECT id, document_id, phone, sms_code, sent_at, confirmed, confirmed_at
	          FROM sms_confirmations
	          WHERE document_id = $1 AND sms_code = $2`
	row := r.DB.QueryRow(query, documentID, code)

	var sms models.SMSConfirmation
	err := row.Scan(&sms.ID, &sms.DocumentID, &sms.Phone, &sms.SMSCode, &sms.SentAt, &sms.Confirmed, &sms.ConfirmedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get sms by doc and code: %w", err)
	}
	return &sms, nil
}

func (r *SMSConfirmationRepository) GetUnconfirmedByDocumentID(documentID int64) ([]*models.SMSConfirmation, error) {
	query := `SELECT id, document_id, phone, sms_code, sent_at, confirmed, confirmed_at
	          FROM sms_confirmations
	          WHERE document_id = $1 AND confirmed = false`
	rows, err := r.DB.Query(query, documentID)
	if err != nil {
		return nil, fmt.Errorf("get unconfirmed sms: %w", err)
	}
	defer rows.Close()

	var confirmations []*models.SMSConfirmation
	for rows.Next() {
		var sms models.SMSConfirmation
		err := rows.Scan(&sms.ID, &sms.DocumentID, &sms.Phone, &sms.SMSCode, &sms.SentAt, &sms.Confirmed, &sms.ConfirmedAt)
		if err != nil {
			return nil, fmt.Errorf("scan unconfirmed sms: %w", err)
		}
		confirmations = append(confirmations, &sms)
	}

	return confirmations, nil
}

func (r *SMSConfirmationRepository) DeleteByDocumentID(documentID int64) error {
	query := `DELETE FROM sms_confirmations WHERE document_id = $1`
	_, err := r.DB.Exec(query, documentID)
	if err != nil {
		return fmt.Errorf("delete sms by docID: %w", err)
	}
	return nil
}
