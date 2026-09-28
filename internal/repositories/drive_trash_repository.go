package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Корзина хранилища (миграция 087). Элемент в корзине вместе со всем
// содержимым помечен deleted_at; trash_root_id указывает на то, что удаляли.

// DriveTrashItem — запись корзины: удалённый файл или папка.
type DriveTrashItem struct {
	ID            int64     `json:"id"`
	Kind          string    `json:"kind"`
	Name          string    `json:"name"`
	MimeType      string    `json:"mime_type"`
	ParentID      *int64    `json:"parent_id"`
	ParentName    string    `json:"parent_name,omitempty"`
	ParentTrashed bool      `json:"parent_trashed,omitempty"`
	SizeBytes     int64     `json:"size_bytes"`
	Files         int64     `json:"files"`
	DeletedAt     time.Time `json:"deleted_at"`
	DeletedByName string    `json:"deleted_by_name,omitempty"`
}

// TrashNode переносит узел со всем неудалённым содержимым в корзину.
// Содержимое, удалённое раньше отдельно, остаётся своей записью в корзине.
func (r *driveRepository) TrashNode(ctx context.Context, id int64, userID int) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		WITH RECURSIVE sub AS (
			SELECT id FROM drive_nodes WHERE id = $1 AND deleted_at IS NULL
			UNION ALL
			SELECT c.id FROM drive_nodes c JOIN sub s ON c.parent_id = s.id
			WHERE c.deleted_at IS NULL
		)
		UPDATE drive_nodes
		SET deleted_at = NOW(), deleted_by = $2, trash_root_id = $1
		WHERE id IN (SELECT id FROM sub)
	`, id, userID)
	if err != nil {
		return 0, fmt.Errorf("trash drive node: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return 0, sql.ErrNoRows
	}
	return n, nil
}

// ListTrash — записи корзины, свежие сверху. Размер и число файлов — всего,
// что удалено вместе с записью.
func (r *driveRepository) ListTrash(ctx context.Context) ([]DriveTrashItem, error) {
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT n.id, n.kind, n.name, n.mime_type, n.parent_id,
		       COALESCE(p.name, ''), (p.deleted_at IS NOT NULL),
		       (SELECT COALESCE(SUM(t.size_bytes) FILTER (WHERE t.kind = 'file'), 0)
		        FROM drive_nodes t WHERE t.trash_root_id = n.id),
		       (SELECT count(*) FILTER (WHERE t.kind = 'file')
		        FROM drive_nodes t WHERE t.trash_root_id = n.id),
		       n.deleted_at, %s
		FROM drive_nodes n
		LEFT JOIN drive_nodes p ON p.id = n.parent_id
		LEFT JOIN users du ON du.id = n.deleted_by
		WHERE n.deleted_at IS NOT NULL AND n.trash_root_id = n.id
		ORDER BY n.deleted_at DESC, n.id DESC
	`, fmt.Sprintf(driveUserNameSQL, "du")))
	if err != nil {
		return nil, fmt.Errorf("list drive trash: %w", err)
	}
	defer rows.Close()
	out := make([]DriveTrashItem, 0)
	for rows.Next() {
		var (
			it       DriveTrashItem
			parentID sql.NullInt64
			pTrashed sql.NullBool
		)
		if err := rows.Scan(&it.ID, &it.Kind, &it.Name, &it.MimeType, &parentID,
			&it.ParentName, &pTrashed, &it.SizeBytes, &it.Files, &it.DeletedAt, &it.DeletedByName); err != nil {
			return nil, fmt.Errorf("scan drive trash: %w", err)
		}
		if parentID.Valid {
			v := parentID.Int64
			it.ParentID = &v
		}
		it.ParentTrashed = pTrashed.Valid && pTrashed.Bool
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetTrashRoot — запись корзины по id. parentLive — жива ли исходная папка:
// если её тоже удалили, восстанавливаем в корень.
func (r *driveRepository) GetTrashRoot(ctx context.Context, id int64) (name string, parentID *int64, parentLive bool, err error) {
	var (
		pid  sql.NullInt64
		live sql.NullBool
	)
	err = r.db.QueryRowContext(ctx, `
		SELECT n.name, n.parent_id, (p.id IS NOT NULL AND p.deleted_at IS NULL)
		FROM drive_nodes n
		LEFT JOIN drive_nodes p ON p.id = n.parent_id
		WHERE n.id = $1 AND n.deleted_at IS NOT NULL AND n.trash_root_id = n.id
	`, id).Scan(&name, &pid, &live)
	if err != nil {
		return "", nil, false, err
	}
	if pid.Valid {
		v := pid.Int64
		parentID = &v
	}
	return name, parentID, live.Valid && live.Bool, nil
}

// RestoreNode возвращает запись корзины в папку parentID под именем name.
func (r *driveRepository) RestoreNode(ctx context.Context, id int64, parentID *int64, name string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("restore drive node: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		UPDATE drive_nodes
		SET parent_id = $2, name = $3, deleted_at = NULL, deleted_by = NULL, trash_root_id = NULL, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NOT NULL AND trash_root_id = $1
	`, id, nullableID(parentID), name)
	if IsSQLState(err, SQLStateUniqueViolation) {
		return ErrDriveNameTaken
	}
	if err != nil {
		return fmt.Errorf("restore drive node: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE drive_nodes SET deleted_at = NULL, deleted_by = NULL, trash_root_id = NULL
		WHERE trash_root_id = $1
	`, id); err != nil {
		return fmt.Errorf("restore drive subtree: %w", err)
	}
	return tx.Commit()
}

// ListTrashRootIDs — все записи корзины (для «Очистить корзину»).
func (r *driveRepository) ListTrashRootIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id FROM drive_nodes WHERE deleted_at IS NOT NULL AND trash_root_id = id ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list drive trash ids: %w", err)
	}
	defer rows.Close()
	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
