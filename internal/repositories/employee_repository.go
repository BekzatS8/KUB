package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"turcompany/internal/models"
)

// EmployeeRepository — раздел «Сотрудники» и кадровые документы (ТЗ «доработки
// CRM КУБ», п.1–2).
type EmployeeRepository struct {
	db *sql.DB
}

func NewEmployeeRepository(db *sql.DB) *EmployeeRepository {
	return &EmployeeRepository{db: db}
}

// ErrEmployeeIINTaken — сотрудник с таким ИИН уже есть.
var ErrEmployeeIINTaken = errors.New("employee iin already exists")

const employeeSelect = `
	SELECT e.id, e.full_name, to_char(e.birth_date, 'YYYY-MM-DD'), e.iin, e.address, e.phone, COALESCE(e.email, ''),
	       e.emergency_name, e.emergency_relation, e.emergency_phone, e.position, e.salary,
	       to_char(e.hired_at, 'YYYY-MM-DD'), COALESCE(to_char(e.fired_at, 'YYYY-MM-DD'), ''), e.employee_type,
	       COALESCE(e.photo_key, ''), e.user_id,
	       COALESCE(NULLIF(TRIM(CONCAT_WS(' ', u.last_name, u.first_name)), ''), u.email, ''),
	       e.branch_id, COALESCE(b.name, ''), e.created_at, e.updated_at
	FROM employees e
	LEFT JOIN users u ON u.id = e.user_id
	LEFT JOIN branches b ON b.id = e.branch_id
`

func scanEmployee(row interface{ Scan(...any) error }) (*models.Employee, error) {
	var e models.Employee
	var userID, branchID sql.NullInt64
	if err := row.Scan(&e.ID, &e.FullName, &e.BirthDate, &e.IIN, &e.Address, &e.Phone, &e.Email,
		&e.EmergencyName, &e.EmergencyRelation, &e.EmergencyPhone, &e.Position, &e.Salary,
		&e.HiredAt, &e.FiredAt, &e.Type, &e.PhotoKey, &userID, &e.UserName,
		&branchID, &e.BranchName, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	e.UserID = nullIntPtr(userID)
	e.BranchID = nullIntPtr(branchID)
	e.HasPhoto = e.PhotoKey != ""
	return &e, nil
}

func employeeWhere(f models.EmployeeFilter) (string, []any) {
	where := []string{"e.deleted_at IS NULL"}
	args := []any{}
	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		n := len(args)
		cond := fmt.Sprintf("(LOWER(e.full_name) LIKE $%d OR e.iin LIKE $%d", n, n)
		if digits := digitsOnly(q); len(digits) >= 3 {
			args = append(args, "%"+digits+"%")
			cond += fmt.Sprintf(" OR regexp_replace(e.phone, '\\D', '', 'g') LIKE $%d", len(args))
		}
		where = append(where, cond+")")
	}
	if f.Type != "" {
		args = append(args, f.Type)
		where = append(where, fmt.Sprintf("e.employee_type = $%d", len(args)))
	} else if !f.IncludeFired {
		where = append(where, "e.employee_type <> 'fired'")
	}
	if p := strings.TrimSpace(f.Position); p != "" {
		args = append(args, strings.ToLower(p))
		where = append(where, fmt.Sprintf("LOWER(e.position) = $%d", len(args)))
	}
	return strings.Join(where, " AND "), args
}

func employeeOrder(f models.EmployeeFilter) string {
	col := "LOWER(e.full_name)"
	switch f.SortBy {
	case "hired_at":
		col = "e.hired_at"
	case "position":
		col = "LOWER(e.position)"
	case "salary":
		col = "e.salary"
	case "created_at":
		col = "e.created_at"
	case "birth_date":
		col = "e.birth_date"
	}
	dir := "ASC"
	if strings.EqualFold(f.Order, "desc") {
		dir = "DESC"
	}
	return col + " " + dir + ", e.id " + dir
}

func (r *EmployeeRepository) List(ctx context.Context, f models.EmployeeFilter) ([]*models.Employee, int, error) {
	where, args := employeeWhere(f)
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM employees e WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count employees: %w", err)
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	args = append(args, limit, f.Offset)
	q := employeeSelect + " WHERE " + where + " ORDER BY " + employeeOrder(f) +
		fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list employees: %w", err)
	}
	defer rows.Close()
	out := []*models.Employee{}
	for rows.Next() {
		e, err := scanEmployee(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func (r *EmployeeRepository) GetByID(ctx context.Context, id int) (*models.Employee, error) {
	e, err := scanEmployee(r.db.QueryRowContext(ctx, employeeSelect+" WHERE e.id = $1 AND e.deleted_at IS NULL", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

// Positions — должности для фильтра.
func (r *EmployeeRepository) Positions(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT TRIM(position) FROM employees
		WHERE deleted_at IS NULL AND TRIM(position) <> ''
		ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func nullDate(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func isUniqueIINViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505" && strings.Contains(pqErr.Constraint, "iin")
}

func (r *EmployeeRepository) Create(ctx context.Context, e *models.Employee, createdBy int) (int, error) {
	var id int
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO employees (full_name, birth_date, iin, address, phone, email, emergency_name, emergency_relation,
		                       emergency_phone, position, salary, hired_at, fired_at, employee_type, user_id, branch_id, created_by)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NULLIF($17, 0))
		RETURNING id
	`, e.FullName, e.BirthDate, e.IIN, e.Address, e.Phone, e.Email, e.EmergencyName, e.EmergencyRelation,
		e.EmergencyPhone, e.Position, e.Salary, e.HiredAt, nullDate(e.FiredAt), e.Type, e.UserID, e.BranchID, createdBy).Scan(&id)
	if err != nil {
		if isUniqueIINViolation(err) {
			return 0, ErrEmployeeIINTaken
		}
		return 0, fmt.Errorf("create employee: %w", err)
	}
	return id, nil
}

func (r *EmployeeRepository) Update(ctx context.Context, e *models.Employee) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE employees
		SET full_name = $2, birth_date = $3, iin = $4, address = $5, phone = $6, email = NULLIF($7, ''),
		    emergency_name = $8, emergency_relation = $9, emergency_phone = $10, position = $11, salary = $12,
		    hired_at = $13, fired_at = $14, employee_type = $15, user_id = $16, branch_id = $17, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, e.ID, e.FullName, e.BirthDate, e.IIN, e.Address, e.Phone, e.Email, e.EmergencyName, e.EmergencyRelation,
		e.EmergencyPhone, e.Position, e.Salary, e.HiredAt, nullDate(e.FiredAt), e.Type, e.UserID, e.BranchID)
	if err != nil {
		if isUniqueIINViolation(err) {
			return ErrEmployeeIINTaken
		}
		return fmt.Errorf("update employee: %w", err)
	}
	return nil
}

func (r *EmployeeRepository) SetPhoto(ctx context.Context, id int, key string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE employees SET photo_key = NULLIF($2, ''), updated_at = NOW() WHERE id = $1`, id, key)
	return err
}

// ─── Шаблоны кадровых документов ────────────────────────────────────────────

func (r *EmployeeRepository) ListTemplates(ctx context.Context) ([]*models.HRTemplate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, kind, COALESCE(original_name, ''), file_key, uploaded_by, created_at
		FROM hr_document_templates WHERE deleted_at IS NULL
		ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("list hr templates: %w", err)
	}
	defer rows.Close()
	out := []*models.HRTemplate{}
	for rows.Next() {
		var t models.HRTemplate
		var by sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Name, &t.Kind, &t.OriginalName, &t.FileKey, &by, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.UploadedBy = nullIntPtr(by)
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (r *EmployeeRepository) GetTemplate(ctx context.Context, id int) (*models.HRTemplate, error) {
	var t models.HRTemplate
	var by sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, kind, COALESCE(original_name, ''), file_key, uploaded_by, created_at
		FROM hr_document_templates WHERE id = $1 AND deleted_at IS NULL`, id).
		Scan(&t.ID, &t.Name, &t.Kind, &t.OriginalName, &t.FileKey, &by, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.UploadedBy = nullIntPtr(by)
	return &t, nil
}

func (r *EmployeeRepository) CreateTemplate(ctx context.Context, t *models.HRTemplate) (int, error) {
	var id int
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO hr_document_templates (name, kind, file_key, original_name, uploaded_by)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5) RETURNING id`,
		t.Name, t.Kind, t.FileKey, t.OriginalName, t.UploadedBy).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create hr template: %w", err)
	}
	return id, nil
}

func (r *EmployeeRepository) DeleteTemplate(ctx context.Context, id int) error {
	_, err := r.db.ExecContext(ctx, `UPDATE hr_document_templates SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id)
	return err
}

// ─── Кадровые документы сотрудника ──────────────────────────────────────────

const employeeDocSelect = `
	SELECT id, COALESCE(employee_id, 0), hr_template_id, COALESCE(title, ''), status, created_at,
	       sent_at, COALESCE(sent_via, ''), COALESCE(sent_to, ''), signed_at, revoked_at,
	       COALESCE(file_path_pdf, ''), COALESCE(file_path_docx, ''), COALESCE(sign_metadata, '')
	FROM documents
`

func scanEmployeeDoc(row interface{ Scan(...any) error }) (*models.EmployeeDocument, error) {
	var d models.EmployeeDocument
	var tpl sql.NullInt64
	var sentAt, signedAt, revokedAt sql.NullTime
	if err := row.Scan(&d.ID, &d.EmployeeID, &tpl, &d.Title, &d.Status, &d.CreatedAt,
		&sentAt, &d.SentVia, &d.SentTo, &signedAt, &revokedAt,
		&d.FilePathPdf, &d.FilePathDocx, &d.SignMetadata); err != nil {
		return nil, err
	}
	d.TemplateID = nullIntPtr(tpl)
	d.SentAt = nullTimePtr(sentAt)
	d.SignedAt = nullTimePtr(signedAt)
	d.RevokedAt = nullTimePtr(revokedAt)
	d.HasSignedPDF = d.Status == "signed" && strings.Contains(d.SignMetadata, "signed_pdf_path")
	return &d, nil
}

func nullTimePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func (r *EmployeeRepository) ListDocuments(ctx context.Context, employeeID int) ([]*models.EmployeeDocument, error) {
	rows, err := r.db.QueryContext(ctx, employeeDocSelect+`
		WHERE employee_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC, id DESC`, employeeID)
	if err != nil {
		return nil, fmt.Errorf("list employee documents: %w", err)
	}
	defer rows.Close()
	out := []*models.EmployeeDocument{}
	for rows.Next() {
		d, err := scanEmployeeDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *EmployeeRepository) GetDocument(ctx context.Context, employeeID int, docID int64) (*models.EmployeeDocument, error) {
	d, err := scanEmployeeDoc(r.db.QueryRowContext(ctx, employeeDocSelect+`
		WHERE id = $1 AND employee_id = $2 AND deleted_at IS NULL`, docID, employeeID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return d, err
}

// CreateDocument — черновик кадрового документа (scope = 'hr').
func (r *EmployeeRepository) CreateDocument(ctx context.Context, employeeID int, templateID int, title, docxKey, pdfKey string, createdBy int) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO documents (deal_id, doc_type, file_path, file_path_docx, file_path_pdf, status, created_by, scope, title,
		                       employee_id, hr_template_id)
		VALUES (NULL, 'hr_template', $1::text, $2::text, $1::text, 'draft', NULLIF($3::int, 0), 'hr', $4::text, $5::int, $6::int)
		RETURNING id`, pdfKey, docxKey, createdBy, title, employeeID, templateID).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create employee document: %w", err)
	}
	return id, nil
}

func (r *EmployeeRepository) MarkSent(ctx context.Context, docID int64, via, to string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE documents SET status = 'sent_for_signature', sent_at = NOW(), sent_via = $2, sent_to = $3, revoked_at = NULL
		WHERE id = $1`, docID, via, to)
	return err
}

// Revoke — отзыв неподписанного документа: статус «Отозван», ожидающие коды
// подтверждения гасятся, чтобы по старой ссылке подписать было нельзя.
func (r *EmployeeRepository) Revoke(ctx context.Context, docID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE documents SET status = 'revoked', revoked_at = NOW() WHERE id = $1 AND status <> 'signed'`, docID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE signature_confirmations SET status = 'cancelled' WHERE document_id = $1 AND status = 'pending'`, docID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sign_sessions SET status = 'expired' WHERE document_id = $1 AND status NOT IN ('signed', 'expired')`, docID); err != nil {
		return err
	}
	return tx.Commit()
}
