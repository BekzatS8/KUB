package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"turcompany/internal/models"
)

// ErrDriveNameTaken — в папке уже есть элемент с таким именем (без учёта регистра).
var ErrDriveNameTaken = errors.New("drive: name already exists in this folder")

// DeletedDriveObject — файл, удалённый из базы вместе с папкой. Объекты в S3
// сервис удаляет после коммита, поэтому ключи собираются заранее.
type DeletedDriveObject struct {
	NodeID     int64
	StorageKey string
}

type DriveRepository interface {
	CreateNode(ctx context.Context, n *models.DriveNode) error
	GetNode(ctx context.Context, id int64) (*models.DriveNode, error)
	ListChildren(ctx context.Context, parentID *int64) ([]models.DriveNode, error)
	RenameNode(ctx context.Context, id int64, name string) error
	DeleteNode(ctx context.Context, id int64) ([]DeletedDriveObject, error)
	Ancestors(ctx context.Context, id int64, userID int) ([]DriveAncestor, error)
	CanAccess(ctx context.Context, nodeID int64, userID int) (bool, error)
	SharedRoots(ctx context.Context, userID int) ([]models.DriveNode, error)
	ListShares(ctx context.Context, nodeID int64) ([]models.DriveShare, error)
	UpsertShares(ctx context.Context, nodeID int64, userIDs []int, expiresAt *time.Time, createdBy int) error
	DeleteShare(ctx context.Context, shareID int64) error
	ListUsers(ctx context.Context) ([]models.DriveUser, error)
	UserHasRole(ctx context.Context, userID, roleID int) (bool, error)
	Totals(ctx context.Context) (totalBytes int64, files int64, err error)
}

// DriveAncestor — звено пути; SharedWithUser — у пользователя есть активный
// доступ именно к этому звену (нужно, чтобы обрезать путь до «корня» доступа).
type DriveAncestor struct {
	ID             int64
	Name           string
	SharedWithUser bool
}

type driveRepository struct {
	db *sql.DB
}

func NewDriveRepository(db *sql.DB) DriveRepository {
	return &driveRepository{db: db}
}

// driveUserNameSQL — ФИО из users с откатом на почту, как в остальной системе.
const driveUserNameSQL = `COALESCE(NULLIF(BTRIM(CONCAT_WS(' ', %[1]s.last_name, %[1]s.first_name)), ''), %[1]s.email, '')`

// activeShareSQL — доступ действует, пока не истёк срок (NULL — бессрочно).
const activeShareSQL = `(%[1]s.expires_at IS NULL OR %[1]s.expires_at > NOW())`

var driveNodeSelect = fmt.Sprintf(`
	SELECT n.id, n.parent_id, n.kind, n.name, COALESCE(n.storage_key, ''), n.size_bytes, n.mime_type,
	       n.created_by, %s, n.created_at, n.updated_at,
	       (SELECT count(*) FROM drive_shares s WHERE s.node_id = n.id AND %s),
	       (SELECT count(*) FROM drive_nodes c WHERE c.parent_id = n.id)
	FROM drive_nodes n
	LEFT JOIN users cu ON cu.id = n.created_by`,
	fmt.Sprintf(driveUserNameSQL, "cu"), fmt.Sprintf(activeShareSQL, "s"))

// Папки выше файлов, внутри — по имени без учёта регистра, как в проводнике.
const driveNodeOrder = ` ORDER BY (n.kind = 'folder') DESC, lower(n.name), n.id`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDriveNode(row rowScanner, extra ...any) (*models.DriveNode, error) {
	var (
		n         models.DriveNode
		parentID  sql.NullInt64
		createdBy sql.NullInt64
		shares    int64
		children  int64
	)
	dest := []any{
		&n.ID, &parentID, &n.Kind, &n.Name, &n.StorageKey, &n.SizeBytes, &n.MimeType,
		&createdBy, &n.CreatedByName, &n.CreatedAt, &n.UpdatedAt, &shares, &children,
	}
	dest = append(dest, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	if parentID.Valid {
		v := parentID.Int64
		n.ParentID = &v
	}
	if createdBy.Valid {
		v := int(createdBy.Int64)
		n.CreatedBy = &v
	}
	n.SharesCount = int(shares)
	n.ChildrenCount = int(children)
	return &n, nil
}

func nullableID(id *int64) sql.NullInt64 {
	if id == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *id, Valid: true}
}

func (r *driveRepository) CreateNode(ctx context.Context, n *models.DriveNode) error {
	var storageKey sql.NullString
	if n.StorageKey != "" {
		storageKey = sql.NullString{String: n.StorageKey, Valid: true}
	}
	var createdBy sql.NullInt64
	if n.CreatedBy != nil {
		createdBy = sql.NullInt64{Int64: int64(*n.CreatedBy), Valid: true}
	}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO drive_nodes (parent_id, kind, name, storage_key, size_bytes, mime_type, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`, nullableID(n.ParentID), n.Kind, n.Name, storageKey, n.SizeBytes, n.MimeType, createdBy,
	).Scan(&n.ID, &n.CreatedAt, &n.UpdatedAt)
	if IsSQLState(err, SQLStateUniqueViolation) {
		return ErrDriveNameTaken
	}
	if IsSQLState(err, SQLStateForeignKey) {
		// Родительская папка удалена между проверкой и вставкой.
		return sql.ErrNoRows
	}
	if err != nil {
		return fmt.Errorf("create drive node: %w", err)
	}
	return nil
}

func (r *driveRepository) GetNode(ctx context.Context, id int64) (*models.DriveNode, error) {
	n, err := scanDriveNode(r.db.QueryRowContext(ctx, driveNodeSelect+` WHERE n.id = $1`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("get drive node: %w", err)
	}
	return n, nil
}

func (r *driveRepository) ListChildren(ctx context.Context, parentID *int64) ([]models.DriveNode, error) {
	rows, err := r.db.QueryContext(ctx,
		driveNodeSelect+` WHERE n.parent_id IS NOT DISTINCT FROM $1::bigint`+driveNodeOrder,
		nullableID(parentID))
	if err != nil {
		return nil, fmt.Errorf("list drive children: %w", err)
	}
	defer rows.Close()
	out := make([]models.DriveNode, 0)
	for rows.Next() {
		n, err := scanDriveNode(rows)
		if err != nil {
			return nil, fmt.Errorf("scan drive node: %w", err)
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

func (r *driveRepository) RenameNode(ctx context.Context, id int64, name string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE drive_nodes SET name = $2, updated_at = NOW() WHERE id = $1`, id, name)
	if IsSQLState(err, SQLStateUniqueViolation) {
		return ErrDriveNameTaken
	}
	if err != nil {
		return fmt.Errorf("rename drive node: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteNode удаляет узел вместе с содержимым и возвращает файлы, чьи объекты
// нужно убрать из S3. Ключи собираются в той же транзакции до удаления, иначе
// после каскада их уже не найти и объекты останутся в бакете навсегда.
func (r *driveRepository) DeleteNode(ctx context.Context, id int64) ([]DeletedDriveObject, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT id, kind, storage_key FROM drive_nodes WHERE id = $1
			UNION ALL
			SELECT c.id, c.kind, c.storage_key
			FROM drive_nodes c
			JOIN subtree s ON c.parent_id = s.id
		)
		SELECT id, COALESCE(storage_key, '') FROM subtree WHERE kind = 'file'
	`, id)
	if err != nil {
		return nil, fmt.Errorf("collect drive subtree: %w", err)
	}
	objects := make([]DeletedDriveObject, 0)
	for rows.Next() {
		var o DeletedDriveObject
		if err := rows.Scan(&o.NodeID, &o.StorageKey); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan drive subtree: %w", err)
		}
		objects = append(objects, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM drive_nodes WHERE id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("delete drive node: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return nil, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return objects, nil
}

// Ancestors — путь от корня до узла включительно.
func (r *driveRepository) Ancestors(ctx context.Context, id int64, userID int) ([]DriveAncestor, error) {
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		WITH RECURSIVE chain AS (
			SELECT id, parent_id, name, 0 AS depth FROM drive_nodes WHERE id = $1
			UNION ALL
			SELECT p.id, p.parent_id, p.name, c.depth + 1
			FROM drive_nodes p
			JOIN chain c ON p.id = c.parent_id
		)
		SELECT c.id, c.name,
		       EXISTS (SELECT 1 FROM drive_shares s
		               WHERE s.node_id = c.id AND s.user_id = $2 AND %s)
		FROM chain c
		ORDER BY c.depth DESC
	`, fmt.Sprintf(activeShareSQL, "s")), id, userID)
	if err != nil {
		return nil, fmt.Errorf("drive ancestors: %w", err)
	}
	defer rows.Close()
	out := make([]DriveAncestor, 0)
	for rows.Next() {
		var a DriveAncestor
		if err := rows.Scan(&a.ID, &a.Name, &a.SharedWithUser); err != nil {
			return nil, fmt.Errorf("scan drive ancestor: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CanAccess — есть ли у пользователя активный доступ к узлу или к любой папке
// выше него. Доступ к папке наследуется всем её содержимым.
func (r *driveRepository) CanAccess(ctx context.Context, nodeID int64, userID int) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx, fmt.Sprintf(`
		WITH RECURSIVE chain AS (
			SELECT id, parent_id FROM drive_nodes WHERE id = $1
			UNION ALL
			SELECT p.id, p.parent_id
			FROM drive_nodes p
			JOIN chain c ON p.id = c.parent_id
		)
		SELECT EXISTS (
			SELECT 1 FROM drive_shares s
			JOIN chain c ON c.id = s.node_id
			WHERE s.user_id = $2 AND %s
		)
	`, fmt.Sprintf(activeShareSQL, "s")), nodeID, userID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("drive access check: %w", err)
	}
	return ok, nil
}

// SharedRoots — «Доступные мне»: узлы, к которым пользователю выдан доступ.
//
// Узел не показывается отдельно, если доступ к нему уже даёт общая папка выше:
// иначе файл из расшаренной папки, выданный ещё и точечно, висел бы в списке
// дважды — и внутри папки, и в корне.
func (r *driveRepository) SharedRoots(ctx context.Context, userID int) ([]models.DriveNode, error) {
	query := fmt.Sprintf(`
		SELECT n.id, n.parent_id, n.kind, n.name, COALESCE(n.storage_key, ''), n.size_bytes, n.mime_type,
		       n.created_by, %[1]s, n.created_at, n.updated_at,
		       0,
		       (SELECT count(*) FROM drive_nodes c WHERE c.parent_id = n.id),
		       sh.expires_at
		FROM drive_shares sh
		JOIN drive_nodes n ON n.id = sh.node_id
		LEFT JOIN users cu ON cu.id = n.created_by
		WHERE sh.user_id = $1
		  AND %[2]s
		  AND NOT EXISTS (
		      WITH RECURSIVE up(id) AS (
		          SELECT n.parent_id
		          UNION ALL
		          SELECT p.parent_id FROM drive_nodes p JOIN up ON p.id = up.id
		      )
		      SELECT 1 FROM up
		      JOIN drive_shares s2 ON s2.node_id = up.id
		      WHERE s2.user_id = $1 AND %[3]s
		  )
	`, fmt.Sprintf(driveUserNameSQL, "cu"), fmt.Sprintf(activeShareSQL, "sh"), fmt.Sprintf(activeShareSQL, "s2"))

	rows, err := r.db.QueryContext(ctx, query+driveNodeOrder, userID)
	if err != nil {
		return nil, fmt.Errorf("drive shared roots: %w", err)
	}
	defer rows.Close()
	out := make([]models.DriveNode, 0)
	for rows.Next() {
		var expires sql.NullTime
		n, err := scanDriveNode(rows, &expires)
		if err != nil {
			return nil, fmt.Errorf("scan shared root: %w", err)
		}
		if expires.Valid {
			t := expires.Time
			n.ShareExpiresAt = &t
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

func (r *driveRepository) ListShares(ctx context.Context, nodeID int64) ([]models.DriveShare, error) {
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT s.id, s.node_id, s.user_id, %s, COALESCE(u.email, ''),
		       s.expires_at, NOT %s, s.created_by, %s, s.created_at
		FROM drive_shares s
		JOIN users u ON u.id = s.user_id
		LEFT JOIN users cb ON cb.id = s.created_by
		WHERE s.node_id = $1
		ORDER BY lower(%s)
	`, fmt.Sprintf(driveUserNameSQL, "u"), fmt.Sprintf(activeShareSQL, "s"),
		fmt.Sprintf(driveUserNameSQL, "cb"), fmt.Sprintf(driveUserNameSQL, "u")), nodeID)
	if err != nil {
		return nil, fmt.Errorf("list drive shares: %w", err)
	}
	defer rows.Close()
	out := make([]models.DriveShare, 0)
	for rows.Next() {
		var (
			sh        models.DriveShare
			expires   sql.NullTime
			createdBy sql.NullInt64
		)
		if err := rows.Scan(&sh.ID, &sh.NodeID, &sh.UserID, &sh.UserName, &sh.UserEmail,
			&expires, &sh.Expired, &createdBy, &sh.CreatedByName, &sh.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan drive share: %w", err)
		}
		if expires.Valid {
			t := expires.Time
			sh.ExpiresAt = &t
		}
		if createdBy.Valid {
			v := int(createdBy.Int64)
			sh.CreatedBy = &v
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

// UpsertShares выдаёт доступ нескольким пользователям разом. Повторная выдача
// тому же пользователю обновляет срок — так продлевают или делают бессрочным.
func (r *driveRepository) UpsertShares(ctx context.Context, nodeID int64, userIDs []int, expiresAt *time.Time, createdBy int) error {
	if len(userIDs) == 0 {
		return nil
	}
	var expires sql.NullTime
	if expiresAt != nil {
		expires = sql.NullTime{Time: *expiresAt, Valid: true}
	}
	ids := make([]int64, 0, len(userIDs))
	for _, id := range userIDs {
		ids = append(ids, int64(id))
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO drive_shares (node_id, user_id, expires_at, created_by)
		SELECT $1, u.id, $3, $4
		FROM users u
		WHERE u.id = ANY($2)
		ON CONFLICT (node_id, user_id) DO UPDATE
		SET expires_at = EXCLUDED.expires_at,
		    created_by = EXCLUDED.created_by,
		    created_at = NOW()
	`, nodeID, pq.Array(ids), expires, createdBy)
	if IsSQLState(err, SQLStateForeignKey) {
		return sql.ErrNoRows
	}
	if err != nil {
		return fmt.Errorf("upsert drive shares: %w", err)
	}
	return nil
}

func (r *driveRepository) DeleteShare(ctx context.Context, shareID int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM drive_shares WHERE id = $1`, shareID)
	if err != nil {
		return fmt.Errorf("delete drive share: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *driveRepository) ListUsers(ctx context.Context) ([]models.DriveUser, error) {
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT u.id, %s, COALESCE(u.email, ''), COALESCE(u.role_id, 0)
		FROM users u
		WHERE COALESCE(u.is_active, TRUE) = TRUE
		ORDER BY lower(%s)
	`, fmt.Sprintf(driveUserNameSQL, "u"), fmt.Sprintf(driveUserNameSQL, "u")))
	if err != nil {
		return nil, fmt.Errorf("list drive users: %w", err)
	}
	defer rows.Close()
	out := make([]models.DriveUser, 0)
	for rows.Next() {
		var u models.DriveUser
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.RoleID); err != nil {
			return nil, fmt.Errorf("scan drive user: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *driveRepository) UserHasRole(ctx context.Context, userID, roleID int) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM users
			WHERE id = $1 AND role_id = $2 AND COALESCE(is_active, TRUE) = TRUE
		)
	`, userID, roleID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("drive role check: %w", err)
	}
	return ok, nil
}

func (r *driveRepository) Totals(ctx context.Context) (int64, int64, error) {
	var totalBytes, files int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(size_bytes), 0), count(*) FILTER (WHERE kind = 'file')
		FROM drive_nodes
	`).Scan(&totalBytes, &files)
	if err != nil {
		return 0, 0, fmt.Errorf("drive totals: %w", err)
	}
	return totalBytes, files, nil
}
