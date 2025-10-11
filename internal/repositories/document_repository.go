package repositories

import (
	"database/sql"
	"fmt"
	"turcompany/internal/models"
)

type DocumentRepository struct {
	db *sql.DB
}

func NewDocumentRepository(db *sql.DB) *DocumentRepository {
	return &DocumentRepository{db: db}
}

func (r *DocumentRepository) Create(doc *models.Document) (int64, error) {
	query := `INSERT INTO documents (deal_id, doc_type, file_path, status, signed_at)
              VALUES ($1, $2, $3, $4, $5) RETURNING id`

	var id int64
	err := r.db.QueryRow(
		query,
		doc.DealID,
		doc.DocType,
		doc.FilePath,
		doc.Status,
		doc.SignedAt,
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("создание документа в БД: %w", err)
	}

	return id, nil
}

func (r *DocumentRepository) GetByID(id int64) (*models.Document, error) {
	query := `SELECT id, deal_id, doc_type, file_path, status, signed_at FROM documents WHERE id = $1`
	row := r.db.QueryRow(query, id)
	var doc models.Document
	err := row.Scan(&doc.ID, &doc.DealID, &doc.DocType, &doc.FilePath, &doc.Status, &doc.SignedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get document: %w", err)
	}
	return &doc, nil
}

func (r *DocumentRepository) Update(doc *models.Document) error {
	query := `UPDATE documents SET deal_id=$1, doc_type=$2, file_path=$3, status=$4, signed_at=$5 WHERE id=$6`
	_, err := r.db.Exec(query, doc.DealID, doc.DocType, doc.FilePath, doc.Status, doc.SignedAt, doc.ID)
	if err != nil {
		return fmt.Errorf("update document: %w", err)
	}
	return nil
}

func (r *DocumentRepository) Delete(id int64) error {
	query := `DELETE FROM documents WHERE id = $1`
	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	return nil
}

func (r *DocumentRepository) ListDocumentsByDeal(dealID int64) ([]*models.Document, error) {
	query := `SELECT id, deal_id, doc_type, file_path, status, signed_at FROM documents WHERE deal_id = $1`
	rows, err := r.db.Query(query, dealID)
	if err != nil {
		return nil, fmt.Errorf("get by deal: %w", err)
	}
	defer rows.Close()

	var docs []*models.Document
	for rows.Next() {
		var doc models.Document
		err := rows.Scan(&doc.ID, &doc.DealID, &doc.DocType, &doc.FilePath, &doc.Status, &doc.SignedAt)
		if err != nil {
			return nil, err
		}
		docs = append(docs, &doc)
	}
	return docs, nil
}

func (r *DocumentRepository) UpdateStatus(id int64, status string) error {
	query := `UPDATE documents SET status = $1 WHERE id = $2`
	_, err := r.db.Exec(query, status, id)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

// Добавим метод для проверки существования лида
func (r *DocumentRepository) LeadExists(id int) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM leads WHERE id = $1)`
	err := r.db.QueryRow(query, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("проверка существования лида: %w", err)
	}
	return exists, nil
}
func (r *DocumentRepository) ListDocuments(limit, offset int) ([]*models.Document, error) {
	query := `SELECT id, deal_id, doc_type, file_path, status, signed_at 
			  FROM documents 
			  ORDER BY signed_at DESC 
			  LIMIT $1 OFFSET $2`

	rows, err := r.db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	var docs []*models.Document
	for rows.Next() {
		var doc models.Document
		if err := rows.Scan(&doc.ID, &doc.DealID, &doc.DocType, &doc.FilePath, &doc.Status, &doc.SignedAt); err != nil {
			return nil, err
		}
		docs = append(docs, &doc)
	}

	return docs, nil
}
