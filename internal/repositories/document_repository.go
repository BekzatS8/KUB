package repositories

import (
	"database/sql"
	"fmt"
	"time"

	"turcompany/internal/models"
)

type DocumentRepository struct{ db *sql.DB }

func NewDocumentRepository(db *sql.DB) *DocumentRepository { return &DocumentRepository{db: db} }

func documentArchiveWhere(scope ArchiveScope) string {
	switch scope {
	case ArchiveScopeArchivedOnly:
		return "is_archived = TRUE"
	case ArchiveScopeAll:
		return "1=1"
	default:
		return "is_archived = FALSE"
	}
}

func (r *DocumentRepository) Create(doc *models.Document) (int64, error) {
	const q = `
		INSERT INTO documents (deal_id, doc_type, file_path, file_path_docx, file_path_pdf, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`
	var id int64
	var createdAt sql.NullTime
	if err := r.db.QueryRow(q, doc.DealID, doc.DocType, doc.FilePath, doc.FilePathDocx, doc.FilePathPdf, doc.Status).Scan(&id, &createdAt); err != nil {
		return 0, fmt.Errorf("create document: %w", err)
	}
	doc.ID = id
	if createdAt.Valid {
		doc.CreatedAt = createdAt.Time
	}
	return id, nil
}

func (r *DocumentRepository) GetByID(id int64) (*models.Document, error) {
	return r.GetByIDWithArchiveScope(id, ArchiveScopeActiveOnly)
}

func (r *DocumentRepository) GetByIDWithArchiveScope(id int64, scope ArchiveScope) (*models.Document, error) {
	const q = `
		SELECT id, deal_id, doc_type, file_path, file_path_docx, file_path_pdf, status,
		       signed_at, created_at, COALESCE(sign_method,''), COALESCE(sign_ip,''),
		       COALESCE(sign_user_agent,''), COALESCE(sign_metadata,''), COALESCE(signed_by,''),
		       is_archived, archived_at, archived_by, COALESCE(archive_reason,'')
		FROM documents
		WHERE id = $1 AND %s`
	var d models.Document
	var signedAt, createdAt, archivedAt sql.NullTime
	var archivedBy sql.NullInt64
	err := r.db.QueryRow(fmt.Sprintf(q, documentArchiveWhere(scope)), id).Scan(&d.ID, &d.DealID, &d.DocType, &d.FilePath, &d.FilePathDocx, &d.FilePathPdf, &d.Status, &signedAt, &createdAt, &d.SignMethod, &d.SignIP, &d.SignUserAgent, &d.SignMetadata, &d.SignedBy, &d.IsArchived, &archivedAt, &archivedBy, &d.ArchiveReason)
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
	if archivedAt.Valid {
		t := archivedAt.Time
		d.ArchivedAt = &t
	}
	if archivedBy.Valid {
		by := int(archivedBy.Int64)
		d.ArchivedBy = &by
	}
	return &d, nil
}

func (r *DocumentRepository) Update(doc *models.Document) error {
	const q = `
		UPDATE documents SET deal_id=$1, doc_type=$2, file_path=$3, file_path_docx=$4, file_path_pdf=$5, status=$6
		WHERE id = $7`
	if _, err := r.db.Exec(q, doc.DealID, doc.DocType, doc.FilePath, doc.FilePathDocx, doc.FilePathPdf, doc.Status, doc.ID); err != nil {
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

func (r *DocumentRepository) Archive(id int64, archivedBy int, reason string) error {
	_, err := r.db.Exec(`
		UPDATE documents
		SET is_archived = TRUE,
		    archived_at = NOW(),
		    archived_by = $2,
		    archive_reason = $3
		WHERE id = $1
	`, id, archivedBy, reason)
	if err != nil {
		return fmt.Errorf("archive document: %w", err)
	}
	return nil
}

func (r *DocumentRepository) Unarchive(id int64) error {
	_, err := r.db.Exec(`
		UPDATE documents
		SET is_archived = FALSE,
		    archived_at = NULL,
		    archived_by = NULL,
		    archive_reason = NULL
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("unarchive document: %w", err)
	}
	return nil
}

func (r *DocumentRepository) ListDocumentsByDeal(dealID int64) ([]*models.Document, error) {
	return r.ListDocumentsByDealWithArchiveScope(dealID, ArchiveScopeActiveOnly)
}

func (r *DocumentRepository) ListDocumentsByDealWithArchiveScope(dealID int64, scope ArchiveScope) ([]*models.Document, error) {
	const q = `
		SELECT id, deal_id, doc_type, file_path, file_path_docx, file_path_pdf, status,
		       signed_at, created_at, COALESCE(sign_method,''), COALESCE(sign_ip,''),
		       COALESCE(sign_user_agent,''), COALESCE(sign_metadata,''), COALESCE(signed_by,''),
		       is_archived, archived_at, archived_by, COALESCE(archive_reason,'')
		FROM documents WHERE deal_id = $1 AND %s ORDER BY id DESC`
	rows, err := r.db.Query(fmt.Sprintf(q, documentArchiveWhere(scope)), dealID)
	if err != nil {
		return nil, fmt.Errorf("list by deal: %w", err)
	}
	defer rows.Close()
	var res []*models.Document
	for rows.Next() {
		var d models.Document
		var signedAt, createdAt, archivedAt sql.NullTime
		var archivedBy sql.NullInt64
		if err := rows.Scan(&d.ID, &d.DealID, &d.DocType, &d.FilePath, &d.FilePathDocx, &d.FilePathPdf, &d.Status, &signedAt, &createdAt, &d.SignMethod, &d.SignIP, &d.SignUserAgent, &d.SignMetadata, &d.SignedBy, &d.IsArchived, &archivedAt, &archivedBy, &d.ArchiveReason); err != nil {
			return nil, err
		}
		if signedAt.Valid {
			d.SignedAt = &signedAt.Time
		}
		if createdAt.Valid {
			d.CreatedAt = createdAt.Time
		}
		if archivedAt.Valid {
			t := archivedAt.Time
			d.ArchivedAt = &t
		}
		if archivedBy.Valid {
			by := int(archivedBy.Int64)
			d.ArchivedBy = &by
		}
		res = append(res, &d)
	}
	return res, rows.Err()
}

func (r *DocumentRepository) UpdateStatus(id int64, status string) error {
	if status == "signed" {
		if _, err := r.db.Exec(`UPDATE documents SET status = $1, signed_at = NOW() WHERE id = $2`, status, id); err != nil {
			return fmt.Errorf("update status: %w", err)
		}
		return nil
	}
	if _, err := r.db.Exec(`UPDATE documents SET status = $1 WHERE id = $2`, status, id); err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

func (r *DocumentRepository) MarkSigned(id int64, signedBy string, signedAt time.Time) error {
	if _, err := r.db.Exec(`UPDATE documents SET status='signed', signed_at=$2, signed_by=NULLIF($3,'') WHERE id=$1`, id, signedAt, signedBy); err != nil {
		return fmt.Errorf("mark signed: %w", err)
	}
	return nil
}

func (r *DocumentRepository) ListDocuments(limit, offset int) ([]*models.Document, error) {
	return r.ListDocumentsWithArchiveScope(limit, offset, ArchiveScopeActiveOnly)
}

func (r *DocumentRepository) ListDocumentsWithArchiveScope(limit, offset int, scope ArchiveScope) ([]*models.Document, error) {
	const q = `
		SELECT id, deal_id, doc_type, file_path, file_path_docx, file_path_pdf, status,
		       signed_at, created_at, COALESCE(sign_method,''), COALESCE(sign_ip,''),
		       COALESCE(sign_user_agent,''), COALESCE(sign_metadata,''), COALESCE(signed_by,''),
		       is_archived, archived_at, archived_by, COALESCE(archive_reason,'')
		FROM documents WHERE %s ORDER BY id DESC LIMIT $1 OFFSET $2`
	rows, err := r.db.Query(fmt.Sprintf(q, documentArchiveWhere(scope)), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()
	var res []*models.Document
	for rows.Next() {
		var d models.Document
		var signedAt, createdAt, archivedAt sql.NullTime
		var archivedBy sql.NullInt64
		if err := rows.Scan(&d.ID, &d.DealID, &d.DocType, &d.FilePath, &d.FilePathDocx, &d.FilePathPdf, &d.Status, &signedAt, &createdAt, &d.SignMethod, &d.SignIP, &d.SignUserAgent, &d.SignMetadata, &d.SignedBy, &d.IsArchived, &archivedAt, &archivedBy, &d.ArchiveReason); err != nil {
			return nil, err
		}
		if signedAt.Valid {
			d.SignedAt = &signedAt.Time
		}
		if createdAt.Valid {
			d.CreatedAt = createdAt.Time
		}
		if archivedAt.Valid {
			t := archivedAt.Time
			d.ArchivedAt = &t
		}
		if archivedBy.Valid {
			by := int(archivedBy.Int64)
			d.ArchivedBy = &by
		}
		res = append(res, &d)
	}
	return res, rows.Err()
}

func (r *DocumentRepository) UpdateSigningMeta(id int64, signMethod, signIP, signUserAgent, signMetadata string) error {
	_, err := r.db.Exec(`
		UPDATE documents
		SET sign_method = NULLIF($1, ''), sign_ip = NULLIF($2, ''), sign_user_agent = NULLIF($3, ''), sign_metadata = NULLIF($4, '')
		WHERE id = $5
	`, signMethod, signIP, signUserAgent, signMetadata, id)
	if err != nil {
		return fmt.Errorf("update signing meta: %w", err)
	}
	return nil
}
