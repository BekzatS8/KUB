package repositories

import (
	"database/sql"
	"fmt"

	"turcompany/internal/models"
)

type DocumentRepository struct{ db *sql.DB }

func NewDocumentRepository(db *sql.DB) *DocumentRepository { return &DocumentRepository{db: db} }

// Create — сохраняем все пути (pdf/docx) + статус.
func (r *DocumentRepository) Create(doc *models.Document) (int64, error) {
	const q = `
		INSERT INTO documents (deal_id, doc_type, file_path, file_path_docx, file_path_pdf, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`
	var id int64
	var createdAt sql.NullTime
	if err := r.db.QueryRow(q,
		doc.DealID,
		doc.DocType,
		doc.FilePath,
		doc.FilePathDocx,
		doc.FilePathPdf,
		doc.Status,
	).Scan(&id, &createdAt); err != nil {
		return 0, fmt.Errorf("create document: %w", err)
	}
	doc.ID = id
	if createdAt.Valid {
		doc.CreatedAt = createdAt.Time
	}
	return id, nil
}

// GetByID — читаем базовые поля + пути; signed_at игнорируем (если не нужен в модели).
func (r *DocumentRepository) GetByID(id int64) (*models.Document, error) {
	const q = `
		SELECT id, deal_id, doc_type, file_path, file_path_docx, file_path_pdf, status, signed_at, created_at
		FROM documents
		WHERE id = $1`
	var d models.Document
	var signedAt sql.NullTime
	var createdAt sql.NullTime
	err := r.db.QueryRow(q, id).Scan(
		&d.ID,
		&d.DealID,
		&d.DocType,
		&d.FilePath,
		&d.FilePathDocx,
		&d.FilePathPdf,
		&d.Status,
		&signedAt,
		&createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	if signedAt.Valid {
		d.SignedAt = &signedAt.Time
	}
	if createdAt.Valid {
		d.CreatedAt = createdAt.Time
	}
	return &d, nil
}

// Update — на случай, если где-то нужно править документ.
func (r *DocumentRepository) Update(doc *models.Document) error {
	const q = `
		UPDATE documents
		SET deal_id = $1,
		    doc_type = $2,
		    file_path = $3,
		    file_path_docx = $4,
		    file_path_pdf = $5,
		    status = $6
		WHERE id = $7`
	if _, err := r.db.Exec(q,
		doc.DealID,
		doc.DocType,
		doc.FilePath,
		doc.FilePathDocx,
		doc.FilePathPdf,
		doc.Status,
		doc.ID,
	); err != nil {
		return fmt.Errorf("update document: %w", err)
	}
	return nil
}

func (r *DocumentRepository) Delete(id int64) error {
	if _, err := r.db.Exec(`DELETE FROM documents WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	return nil
}

func (r *DocumentRepository) ListDocumentsByDeal(dealID int64) ([]*models.Document, error) {
	const q = `
		SELECT id, deal_id, doc_type, file_path, file_path_docx, file_path_pdf, status, signed_at, created_at
		FROM documents
		WHERE deal_id = $1
		ORDER BY id DESC`
	rows, err := r.db.Query(q, dealID)
	if err != nil {
		return nil, fmt.Errorf("list by deal: %w", err)
	}
	defer rows.Close()

	var res []*models.Document
	for rows.Next() {
		var d models.Document
		var signedAt sql.NullTime
		var createdAt sql.NullTime
		if err := rows.Scan(
			&d.ID,
			&d.DealID,
			&d.DocType,
			&d.FilePath,
			&d.FilePathDocx,
			&d.FilePathPdf,
			&d.Status,
			&signedAt,
			&createdAt,
		); err != nil {
			return nil, err
		}
		if signedAt.Valid {
			d.SignedAt = &signedAt.Time
		}
		if createdAt.Valid {
			d.CreatedAt = createdAt.Time
		}
		res = append(res, &d)
	}
	return res, rows.Err()
}

// Если статус "signed" — проставляем signed_at, иначе просто обновляем статус.
func (r *DocumentRepository) UpdateStatus(id int64, status string) error {
	if status == "signed" {
		if _, err := r.db.Exec(
			`UPDATE documents SET status = $1, signed_at = NOW() WHERE id = $2`,
			status, id,
		); err != nil {
			return fmt.Errorf("update status: %w", err)
		}
		return nil
	}

	if _, err := r.db.Exec(
		`UPDATE documents SET status = $1 WHERE id = $2`,
		status, id,
	); err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

func (r *DocumentRepository) ListDocuments(limit, offset int) ([]*models.Document, error) {
	const q = `
		SELECT id, deal_id, doc_type, file_path, file_path_docx, file_path_pdf, status, signed_at, created_at
		FROM documents
		ORDER BY id DESC
		LIMIT $1 OFFSET $2`
	rows, err := r.db.Query(q, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	var res []*models.Document
	for rows.Next() {
		var d models.Document
		var signedAt sql.NullTime
		var createdAt sql.NullTime
		if err := rows.Scan(
			&d.ID,
			&d.DealID,
			&d.DocType,
			&d.FilePath,
			&d.FilePathDocx,
			&d.FilePathPdf,
			&d.Status,
			&signedAt,
			&createdAt,
		); err != nil {
			return nil, err
		}
		if signedAt.Valid {
			d.SignedAt = &signedAt.Time
		}
		if createdAt.Valid {
			d.CreatedAt = createdAt.Time
		}
		res = append(res, &d)
	}
	return res, rows.Err()
}

func (r *DocumentRepository) UpdateSigningMeta(id int64, signMethod, signIP, signUserAgent, signMetadata string) error {
	_, err := r.db.Exec(`
		UPDATE documents
		SET sign_method = NULLIF($1, ''),
		    sign_ip = NULLIF($2, ''),
		    sign_user_agent = NULLIF($3, ''),
		    sign_metadata = NULLIF($4, '')
		WHERE id = $5
	`, signMethod, signIP, signUserAgent, signMetadata, id)
	if err != nil {
		return fmt.Errorf("update signing meta: %w", err)
	}
	return nil
}
