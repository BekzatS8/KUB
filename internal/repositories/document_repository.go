package repositories

import (
	"database/sql"
	"fmt"

	"turcompany/internal/models"
)

type DocumentRepository struct{ db *sql.DB }

func NewDocumentRepository(db *sql.DB) *DocumentRepository { return &DocumentRepository{db: db} }

// Create — сохраняем только базовые поля.
// Колонка signed_at, если есть в БД, просто остаётся NULL (мы её тут не трогаем).
func (r *DocumentRepository) Create(doc *models.Document) (int64, error) {
	const q = `
		INSERT INTO documents (deal_id, doc_type, file_path, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id`
	var id int64
	if err := r.db.QueryRow(q,
		doc.DealID,
		doc.DocType,
		doc.FilePath,
		doc.Status,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("create document: %w", err)
	}
	return id, nil
}

// GetByID — читаем только те же поля, что и сохраняем.
// Если в таблице есть signed_at/created_at — просто не выбираем их.
func (r *DocumentRepository) GetByID(id int64) (*models.Document, error) {
	const q = `
		SELECT id, deal_id, doc_type, file_path, status
		FROM documents
		WHERE id = $1`
	var d models.Document
	err := r.db.QueryRow(q, id).Scan(
		&d.ID,
		&d.DealID,
		&d.DocType,
		&d.FilePath,
		&d.Status,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	return &d, nil
}

// Update — если вдруг где-то понадобилось обновление документа целиком.
func (r *DocumentRepository) Update(doc *models.Document) error {
	const q = `
		UPDATE documents
		SET deal_id = $1,
		    doc_type = $2,
		    file_path = $3,
		    status = $4
		WHERE id = $5`
	if _, err := r.db.Exec(q,
		doc.DealID,
		doc.DocType,
		doc.FilePath,
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
		SELECT id, deal_id, doc_type, file_path, status
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
		if err := rows.Scan(
			&d.ID,
			&d.DealID,
			&d.DocType,
			&d.FilePath,
			&d.Status,
		); err != nil {
			return nil, err
		}
		res = append(res, &d)
	}
	return res, nil
}

func (r *DocumentRepository) UpdateStatus(id int64, status string) error {
	if _, err := r.db.Exec(
		`UPDATE documents SET status = $1 WHERE id = $2`,
		status,
		id,
	); err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

func (r *DocumentRepository) ListDocuments(limit, offset int) ([]*models.Document, error) {
	const q = `
		SELECT id, deal_id, doc_type, file_path, status
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
		if err := rows.Scan(
			&d.ID,
			&d.DealID,
			&d.DocType,
			&d.FilePath,
			&d.Status,
		); err != nil {
			return nil, err
		}
		res = append(res, &d)
	}
	return res, nil
}
