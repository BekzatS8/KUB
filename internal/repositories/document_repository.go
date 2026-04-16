package repositories

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"turcompany/internal/models"
)

type DocumentRepository struct{ db *sql.DB }

func NewDocumentRepository(db *sql.DB) *DocumentRepository { return &DocumentRepository{db: db} }

type DocumentListFilter struct {
	Query      string
	Status     string
	DocType    string
	DealID     *int64
	ClientID   *int64
	ClientType string
	BranchID   *int64
	SortBy     string
	Order      string
}

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

const documentBaseSelect = `
	SELECT dcm.id, dcm.deal_id, dcm.branch_id, COALESCE(br.name,''), dcm.doc_type, dcm.file_path, dcm.file_path_docx, dcm.file_path_pdf, dcm.status,
	       dcm.signed_at, dcm.created_at, COALESCE(dcm.sign_method,''), COALESCE(dcm.sign_ip,''),
	       COALESCE(dcm.sign_user_agent,''), COALESCE(dcm.sign_metadata,''), COALESCE(dcm.signed_by,''),
	       dcm.is_archived, dcm.archived_at, dcm.archived_by, COALESCE(dcm.archive_reason,'')
	FROM documents dcm
	LEFT JOIN deals d ON d.id = dcm.deal_id
	LEFT JOIN clients c ON c.id = d.client_id
	LEFT JOIN branches br ON br.id = dcm.branch_id
`

const documentBaseFrom = `
	FROM documents dcm
	LEFT JOIN deals d ON d.id = dcm.deal_id
	LEFT JOIN clients c ON c.id = d.client_id
	LEFT JOIN branches br ON br.id = dcm.branch_id
`

func scanDocument(scanner interface{ Scan(dest ...any) error }) (*models.Document, error) {
	var d models.Document
	var signedAt, createdAt, archivedAt sql.NullTime
	var archivedBy sql.NullInt64
	var branchID sql.NullInt64
	var branchName sql.NullString
	if err := scanner.Scan(&d.ID, &d.DealID, &branchID, &branchName, &d.DocType, &d.FilePath, &d.FilePathDocx, &d.FilePathPdf, &d.Status, &signedAt, &createdAt, &d.SignMethod, &d.SignIP, &d.SignUserAgent, &d.SignMetadata, &d.SignedBy, &d.IsArchived, &archivedAt, &archivedBy, &d.ArchiveReason); err != nil {
		return nil, err
	}
	if branchID.Valid {
		v := branchID.Int64
		d.BranchID = &v
	}
	if branchName.Valid {
		d.BranchName = branchName.String
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

func (r *DocumentRepository) Create(doc *models.Document) (int64, error) {
	const q = `
		INSERT INTO documents (deal_id, branch_id, doc_type, file_path, file_path_docx, file_path_pdf, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`
	var id int64
	var createdAt sql.NullTime
	if err := r.db.QueryRow(q, doc.DealID, doc.BranchID, doc.DocType, doc.FilePath, doc.FilePathDocx, doc.FilePathPdf, doc.Status).Scan(&id, &createdAt); err != nil {
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
		SELECT id, deal_id, branch_id, doc_type, file_path, file_path_docx, file_path_pdf, status,
		       signed_at, created_at, COALESCE(sign_method,''), COALESCE(sign_ip,''),
		       COALESCE(sign_user_agent,''), COALESCE(sign_metadata,''), COALESCE(signed_by,''),
		       is_archived, archived_at, archived_by, COALESCE(archive_reason,'')
		FROM documents
		WHERE id = $1 AND %s`
	var d models.Document
	var signedAt, createdAt, archivedAt sql.NullTime
	var archivedBy sql.NullInt64
	var branchID sql.NullInt64
	err := r.db.QueryRow(fmt.Sprintf(q, documentArchiveWhere(scope)), id).Scan(&d.ID, &d.DealID, &branchID, &d.DocType, &d.FilePath, &d.FilePathDocx, &d.FilePathPdf, &d.Status, &signedAt, &createdAt, &d.SignMethod, &d.SignIP, &d.SignUserAgent, &d.SignMetadata, &d.SignedBy, &d.IsArchived, &archivedAt, &archivedBy, &d.ArchiveReason)
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
	if branchID.Valid {
		v := branchID.Int64
		d.BranchID = &v
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
		UPDATE documents SET deal_id=$1, branch_id=$2, doc_type=$3, file_path=$4, file_path_docx=$5, file_path_pdf=$6, status=$7
		WHERE id = $8`
	if _, err := r.db.Exec(q, doc.DealID, doc.BranchID, doc.DocType, doc.FilePath, doc.FilePathDocx, doc.FilePathPdf, doc.Status, doc.ID); err != nil {
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
	filter := DocumentListFilter{DealID: &dealID}
	return r.ListDocumentsByDealWithFilterAndArchiveScope(dealID, filter, scope)
}

func (r *DocumentRepository) ListDocumentsByDealWithFilterAndArchiveScope(dealID int64, filter DocumentListFilter, scope ArchiveScope) ([]*models.Document, error) {
	filter.DealID = &dealID
	where, args := buildDocumentListWhere(filter, scope, 1)
	sortExpr, sortOrder := documentSortExpression(filter)
	query := documentBaseSelect + fmt.Sprintf(" WHERE %s ORDER BY %s %s", where, sortExpr, sortOrder)
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list by deal: %w", err)
	}
	defer rows.Close()
	var res []*models.Document
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, d)
	}
	return res, rows.Err()
}

func (r *DocumentRepository) ListDocumentsByDealWithFilterAndArchiveScopePaginated(dealID int64, limit, offset int, filter DocumentListFilter, scope ArchiveScope) ([]*models.Document, error) {
	filter.DealID = &dealID
	return r.ListDocumentsWithFilterAndArchiveScope(limit, offset, filter, scope)
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
	return r.ListDocumentsWithFilterAndArchiveScope(limit, offset, DocumentListFilter{}, scope)
}

func (r *DocumentRepository) ListDocumentsWithFilterAndArchiveScope(limit, offset int, filter DocumentListFilter, scope ArchiveScope) ([]*models.Document, error) {
	where, args := buildDocumentListWhere(filter, scope, 1)
	sortExpr, sortOrder := documentSortExpression(filter)
	args = append(args, limit, offset)
	query := documentBaseSelect + fmt.Sprintf(" WHERE %s ORDER BY %s %s LIMIT $%d OFFSET $%d", where, sortExpr, sortOrder, len(args)-1, len(args))
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()
	var res []*models.Document
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, d)
	}
	return res, rows.Err()
}

func (r *DocumentRepository) CountDocumentsWithFilterAndArchiveScope(filter DocumentListFilter, scope ArchiveScope) (int, error) {
	where, args := buildDocumentListWhere(filter, scope, 1)
	query := "SELECT COUNT(1) " + documentBaseFrom + fmt.Sprintf(" WHERE %s", where)
	var total int
	if err := r.db.QueryRow(query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count documents: %w", err)
	}
	return total, nil
}

func buildDocumentListWhere(filter DocumentListFilter, scope ArchiveScope, startAt int) (string, []any) {
	conditions := []string{strings.ReplaceAll(documentArchiveWhere(scope), "is_archived", "dcm.is_archived")}
	args := make([]any, 0, 8)
	idx := startAt

	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("dcm.status = $%d", idx))
		args = append(args, filter.Status)
		idx++
	}
	if filter.DocType != "" {
		conditions = append(conditions, fmt.Sprintf("dcm.doc_type = $%d", idx))
		args = append(args, filter.DocType)
		idx++
	}
	if filter.DealID != nil {
		conditions = append(conditions, fmt.Sprintf("dcm.deal_id = $%d", idx))
		args = append(args, *filter.DealID)
		idx++
	}
	if filter.ClientID != nil {
		conditions = append(conditions, fmt.Sprintf("d.client_id = $%d", idx))
		args = append(args, *filter.ClientID)
		idx++
	}
	if filter.BranchID != nil {
		conditions = append(conditions, fmt.Sprintf("dcm.branch_id = $%d", idx))
		args = append(args, *filter.BranchID)
		idx++
	}
	if filter.ClientType != "" {
		conditions = append(conditions, fmt.Sprintf("c.client_type = $%d", idx))
		args = append(args, filter.ClientType)
		idx++
	}
	if filter.Query != "" {
		conditions = append(conditions, fmt.Sprintf(`(
			LOWER(COALESCE(dcm.doc_type, '')) LIKE $%d OR
			LOWER(COALESCE(dcm.file_path_docx, dcm.file_path_pdf, dcm.file_path, '')) LIKE $%d OR
			CAST(dcm.deal_id AS TEXT) LIKE $%d OR
			LOWER(COALESCE(c.display_name, c.name, '')) LIKE $%d
		)`, idx, idx, idx, idx))
		args = append(args, "%"+strings.ToLower(filter.Query)+"%")
	}

	return strings.Join(conditions, " AND "), args
}

func documentSortExpression(filter DocumentListFilter) (string, string) {
	order := "DESC"
	if strings.EqualFold(filter.Order, "asc") {
		order = "ASC"
	}
	switch filter.SortBy {
	case "created_at":
		return "dcm.created_at", order
	case "status":
		return "dcm.status", order
	case "doc_type":
		return "dcm.doc_type", order
	case "name":
		return "LOWER(COALESCE(dcm.file_path_docx, dcm.file_path_pdf, dcm.file_path, ''))", order
	default:
		return "dcm.id", order
	}
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
