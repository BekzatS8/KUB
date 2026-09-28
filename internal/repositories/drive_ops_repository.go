package repositories

import (
	"context"
	"database/sql"
	"fmt"
)

// DriveSubtreeNode — узел поддерева для копирования; Depth = 0 у корня.
// Порядок — родители раньше детей.
type DriveSubtreeNode struct {
	ID         int64
	ParentID   *int64
	Kind       string
	Name       string
	StorageKey string
	SizeBytes  int64
	MimeType   string
	Depth      int
}

// DriveFolderStats — содержимое папки со всеми вложенными.
type DriveFolderStats struct {
	Bytes   int64
	Files   int64
	Folders int64
}

// MoveNode переносит узел в другую папку (nil — корень) под именем name.
func (r *driveRepository) MoveNode(ctx context.Context, id int64, parentID *int64, name string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE drive_nodes SET parent_id = $2, name = $3, updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`,
		id, nullableID(parentID), name)
	if IsSQLState(err, SQLStateUniqueViolation) {
		return ErrDriveNameTaken
	}
	if IsSQLState(err, SQLStateForeignKey) {
		return sql.ErrNoRows
	}
	if err != nil {
		return fmt.Errorf("move drive node: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// IsWithin — лежит ли nodeID внутри ancestorID (или совпадает с ним). Нужно,
// чтобы папку нельзя было переместить или скопировать саму в себя.
func (r *driveRepository) IsWithin(ctx context.Context, nodeID, ancestorID int64) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx, `
		WITH RECURSIVE chain AS (
			SELECT id, parent_id FROM drive_nodes WHERE id = $1
			UNION ALL
			SELECT p.id, p.parent_id FROM drive_nodes p JOIN chain c ON p.id = c.parent_id
		)
		SELECT EXISTS (SELECT 1 FROM chain WHERE id = $2)
	`, nodeID, ancestorID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("drive within check: %w", err)
	}
	return ok, nil
}

// Subtree — узел со всем содержимым, родители раньше детей.
func (r *driveRepository) Subtree(ctx context.Context, id int64) ([]DriveSubtreeNode, error) {
	rows, err := r.db.QueryContext(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT id, parent_id, kind, name, storage_key, size_bytes, mime_type, 0 AS depth
			FROM drive_nodes WHERE id = $1 AND deleted_at IS NULL
			UNION ALL
			SELECT c.id, c.parent_id, c.kind, c.name, c.storage_key, c.size_bytes, c.mime_type, s.depth + 1
			FROM drive_nodes c JOIN subtree s ON c.parent_id = s.id
			WHERE c.deleted_at IS NULL
		)
		SELECT id, parent_id, kind, name, COALESCE(storage_key, ''), size_bytes, mime_type, depth
		FROM subtree
		ORDER BY depth, id
	`, id)
	if err != nil {
		return nil, fmt.Errorf("drive subtree: %w", err)
	}
	defer rows.Close()
	out := make([]DriveSubtreeNode, 0)
	for rows.Next() {
		var (
			n        DriveSubtreeNode
			parentID sql.NullInt64
		)
		if err := rows.Scan(&n.ID, &parentID, &n.Kind, &n.Name, &n.StorageKey, &n.SizeBytes, &n.MimeType, &n.Depth); err != nil {
			return nil, fmt.Errorf("scan drive subtree: %w", err)
		}
		if parentID.Valid {
			v := parentID.Int64
			n.ParentID = &v
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// FolderStats — размер и число файлов и папок внутри папки (всех уровней).
func (r *driveRepository) FolderStats(ctx context.Context, id int64) (*DriveFolderStats, error) {
	var st DriveFolderStats
	err := r.db.QueryRowContext(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT id, kind, size_bytes FROM drive_nodes WHERE parent_id = $1 AND deleted_at IS NULL
			UNION ALL
			SELECT c.id, c.kind, c.size_bytes FROM drive_nodes c JOIN subtree s ON c.parent_id = s.id
			WHERE c.deleted_at IS NULL
		)
		SELECT COALESCE(SUM(size_bytes) FILTER (WHERE kind = 'file'), 0),
		       COUNT(*) FILTER (WHERE kind = 'file'),
		       COUNT(*) FILTER (WHERE kind = 'folder')
		FROM subtree
	`, id).Scan(&st.Bytes, &st.Files, &st.Folders)
	if err != nil {
		return nil, fmt.Errorf("drive folder stats: %w", err)
	}
	return &st, nil
}
