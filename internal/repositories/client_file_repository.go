package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"turcompany/internal/models"
)

type ClientFileRepository struct {
	db *sql.DB
}

func NewClientFileRepository(db *sql.DB) *ClientFileRepository {
	return &ClientFileRepository{db: db}
}

func (r *ClientFileRepository) UpsertPrimary(
	ctx context.Context,
	clientID int64,
	category string,
	filePath string,
	mime *string,
	sizeBytes *int64,
	uploadedBy *int64,
) (*models.ClientFile, error) {
	const q = `
		INSERT INTO client_files (client_id, category, file_path, mime, size_bytes, uploaded_by, is_primary)
		VALUES ($1, $2, $3, $4, $5, $6, TRUE)
		ON CONFLICT (client_id, category) WHERE is_primary = TRUE
		DO UPDATE SET
			file_path = EXCLUDED.file_path,
			mime = EXCLUDED.mime,
			size_bytes = EXCLUDED.size_bytes,
			uploaded_by = EXCLUDED.uploaded_by
		RETURNING id, client_id, category, file_path, mime, size_bytes, uploaded_by, created_at, is_primary
	`

	row := r.db.QueryRowContext(ctx, q, clientID, category, filePath, mime, sizeBytes, uploadedBy)
	file, err := scanClientFile(row)
	if err != nil {
		return nil, fmt.Errorf("upsert primary client file: %w", err)
	}
	return file, nil
}

func (r *ClientFileRepository) ListByClient(ctx context.Context, clientID int64) ([]*models.ClientFile, error) {
	const q = `
		SELECT id, client_id, category, file_path, mime, size_bytes, uploaded_by, created_at, is_primary
		FROM client_files
		WHERE client_id = $1
		ORDER BY created_at DESC, id DESC
	`

	rows, err := r.db.QueryContext(ctx, q, clientID)
	if err != nil {
		return nil, fmt.Errorf("list client files by client: %w", err)
	}
	defer rows.Close()

	var res []*models.ClientFile
	for rows.Next() {
		file, err := scanClientFile(rows)
		if err != nil {
			return nil, fmt.Errorf("list client files by client: %w", err)
		}
		res = append(res, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list client files by client: %w", err)
	}
	return res, nil
}

func (r *ClientFileRepository) GetPrimaryByCategory(ctx context.Context, clientID int64, category string) (*models.ClientFile, error) {
	const q = `
		SELECT id, client_id, category, file_path, mime, size_bytes, uploaded_by, created_at, is_primary
		FROM client_files
		WHERE client_id = $1
		  AND category = $2
		  AND is_primary = TRUE
		LIMIT 1
	`

	row := r.db.QueryRowContext(ctx, q, clientID, category)
	file, err := scanClientFile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrClientFileNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get primary client file by category: %w", err)
	}
	return file, nil
}

func scanClientFile(scanner interface{ Scan(dest ...any) error }) (*models.ClientFile, error) {
	var file models.ClientFile
	var mime sql.NullString
	var sizeBytes sql.NullInt64
	var uploadedBy sql.NullInt64

	err := scanner.Scan(
		&file.ID,
		&file.ClientID,
		&file.Category,
		&file.FilePath,
		&mime,
		&sizeBytes,
		&uploadedBy,
		&file.CreatedAt,
		&file.IsPrimary,
	)
	if err != nil {
		return nil, err
	}

	if mime.Valid {
		v := mime.String
		file.Mime = &v
	}
	if sizeBytes.Valid {
		v := sizeBytes.Int64
		file.SizeBytes = &v
	}
	if uploadedBy.Valid {
		v := uploadedBy.Int64
		file.UploadedBy = &v
	}

	return &file, nil
}
