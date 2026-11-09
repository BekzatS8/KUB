package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

// DriveExtrasRepository — пароли на узлы хранилища (ТЗ «доработки CRM КУБ»,
// п.8). Отдельно от DriveRepository, чтобы не расширять его интерфейс.
type DriveExtrasRepository struct {
	db *sql.DB
}

func NewDriveExtrasRepository(db *sql.DB) *DriveExtrasRepository {
	return &DriveExtrasRepository{db: db}
}

// DriveProtectedNode — защищённый паролем узел.
type DriveProtectedNode struct {
	ID   int64
	Name string
	Hash string
}

// ProtectedChain — защищённые узлы среди самого узла и всех его папок-предков.
func (r *DriveExtrasRepository) ProtectedChain(ctx context.Context, nodeID int64) ([]DriveProtectedNode, error) {
	rows, err := r.db.QueryContext(ctx, `
		WITH RECURSIVE chain AS (
			SELECT id, parent_id, name, password_hash FROM drive_nodes WHERE id = $1
			UNION ALL
			SELECT p.id, p.parent_id, p.name, p.password_hash FROM drive_nodes p JOIN chain c ON p.id = c.parent_id
		)
		SELECT id, name, password_hash FROM chain WHERE password_hash IS NOT NULL
	`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("drive protected chain: %w", err)
	}
	defer rows.Close()
	out := []DriveProtectedNode{}
	for rows.Next() {
		var n DriveProtectedNode
		if err := rows.Scan(&n.ID, &n.Name, &n.Hash); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ProtectedAmong — какие из узлов защищены паролем.
func (r *DriveExtrasRepository) ProtectedAmong(ctx context.Context, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM drive_nodes WHERE id = ANY($1) AND password_hash IS NOT NULL`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("drive protected among: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// PasswordHash — хеш пароля узла ("" — пароля нет).
func (r *DriveExtrasRepository) PasswordHash(ctx context.Context, id int64) (string, error) {
	var h sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT password_hash FROM drive_nodes WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", sql.ErrNoRows
	}
	if err != nil {
		return "", err
	}
	return h.String, nil
}

// SetPassword ставит (hash != "") или снимает (hash == "") пароль.
func (r *DriveExtrasRepository) SetPassword(ctx context.Context, id int64, hash string, userID int) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE drive_nodes
		SET password_hash = NULLIF($2, ''),
		    password_set_by = CASE WHEN $2 = '' THEN NULL ELSE NULLIF($3, 0) END,
		    password_set_at = CASE WHEN $2 = '' THEN NULL ELSE NOW() END,
		    updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, id, hash, userID)
	return err
}

// ChildFolder — папка с таким именем внутри parent (nil — корень).
func (r *DriveExtrasRepository) ChildByName(ctx context.Context, parentID *int64, name string) (int64, string, error) {
	var id int64
	var kind string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, kind FROM drive_nodes
		WHERE COALESCE(parent_id, 0) = COALESCE($1, 0) AND lower(name) = lower($2) AND deleted_at IS NULL
		LIMIT 1
	`, nullableID(parentID), name).Scan(&id, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil
	}
	return id, kind, err
}
