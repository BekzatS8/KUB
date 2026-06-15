package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"turcompany/internal/models"
)

type DocumentVersionRepository struct {
	db *sql.DB
}

func NewDocumentVersionRepository(db *sql.DB) *DocumentVersionRepository {
	return &DocumentVersionRepository{db: db}
}

// GetVersions returns all versions for a document, ordered newest first
func (r *DocumentVersionRepository) GetVersions(ctx context.Context, docID int64) ([]*models.DocumentVersion, error) {
	const q = `
		SELECT id, document_id, version, file_path, file_path_pdf, file_path_docx,
		       file_size, mime_type, uploaded_by, comment, created_at
		FROM document_versions
		WHERE document_id = $1
		ORDER BY version DESC
	`
	rows, err := r.db.QueryContext(ctx, q, docID)
	if err != nil {
		return nil, fmt.Errorf("list document versions: %w", err)
	}
	defer rows.Close()

	var res []*models.DocumentVersion
	for rows.Next() {
		v, err := scanDocumentVersion(rows)
		if err != nil {
			return nil, fmt.Errorf("scan document version: %w", err)
		}
		res = append(res, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

// GetLatestVersion returns the latest version number for a document
func (r *DocumentVersionRepository) GetLatestVersion(ctx context.Context, docID int64) (int, error) {
	var version int
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM document_versions WHERE document_id = $1`,
		docID,
	).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("get latest version: %w", err)
	}
	return version, nil
}

// CreateVersion inserts a new version record
func (r *DocumentVersionRepository) CreateVersion(ctx context.Context, v *models.DocumentVersion) (int64, error) {
	const q = `
		INSERT INTO document_versions (document_id, version, file_path, file_path_pdf, file_path_docx, file_size, mime_type, uploaded_by, comment)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at
	`
	var id int64
	var createdAt time.Time
	err := r.db.QueryRowContext(ctx, q,
		v.DocumentID, v.Version, v.FilePath, v.FilePathPdf, v.FilePathDocx,
		v.FileSize, v.MimeType, v.UploadedBy, v.Comment,
	).Scan(&id, &createdAt)
	if err != nil {
		return 0, fmt.Errorf("create document version: %w", err)
	}
	v.ID = id
	v.CreatedAt = createdAt
	return id, nil
}

// GetVersion returns a specific version by ID
func (r *DocumentVersionRepository) GetVersion(ctx context.Context, versionID int64) (*models.DocumentVersion, error) {
	const q = `
		SELECT id, document_id, version, file_path, file_path_pdf, file_path_docx,
		       file_size, mime_type, uploaded_by, comment, created_at
		FROM document_versions
		WHERE id = $1
	`
	row := r.db.QueryRowContext(ctx, q, versionID)
	v, err := scanDocumentVersion(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get document version: %w", err)
	}
	return v, nil
}

// GetVersionByNumber returns a specific version by document_id and version number
func (r *DocumentVersionRepository) GetVersionByNumber(ctx context.Context, docID int64, version int) (*models.DocumentVersion, error) {
	const q = `
		SELECT id, document_id, version, file_path, file_path_pdf, file_path_docx,
		       file_size, mime_type, uploaded_by, comment, created_at
		FROM document_versions
		WHERE document_id = $1 AND version = $2
	`
	row := r.db.QueryRowContext(ctx, q, docID, version)
	v, err := scanDocumentVersion(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get document version by number: %w", err)
	}
	return v, nil
}

func scanDocumentVersion(scanner interface{ Scan(dest ...any) error }) (*models.DocumentVersion, error) {
	var v models.DocumentVersion
	var fileSize sql.NullInt64
	var mimeType, comment sql.NullString
	var uploadedBy sql.NullInt64
	var createdAt time.Time

	if err := scanner.Scan(
		&v.ID, &v.DocumentID, &v.Version, &v.FilePath, &v.FilePathPdf, &v.FilePathDocx,
		&fileSize, &mimeType, &uploadedBy, &comment, &createdAt,
	); err != nil {
		return nil, err
	}
	if fileSize.Valid {
		v.FileSize = &fileSize.Int64
	}
	if mimeType.Valid {
		v.MimeType = &mimeType.String
	}
	if uploadedBy.Valid {
		by := int(uploadedBy.Int64)
		v.UploadedBy = &by
	}
	if comment.Valid {
		v.Comment = &comment.String
	}
	v.CreatedAt = createdAt
	return &v, nil
}
