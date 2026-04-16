package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/lib/pq"

	"turcompany/internal/models"
)

type LeadRepository struct {
	db *sql.DB
}

type LeadListFilter struct {
	Query       string
	Status      string
	StatusGroup string
	SortBy      string
	Order       string
	BranchID    *int
}

type ArchiveScope string

const (
	ArchiveScopeActiveOnly   ArchiveScope = "active"
	ArchiveScopeArchivedOnly ArchiveScope = "archived"
	ArchiveScopeAll          ArchiveScope = "all"
)

type leadRowScanner interface {
	Scan(dest ...any) error
}

func NewLeadRepository(db *sql.DB) *LeadRepository {
	if db == nil {
		log.Fatalf("received nil database connection")
	}
	return &LeadRepository{db: db}
}

func normalizeLeadStatus(status sql.NullString) string {
	if status.Valid && status.String != "" {
		return status.String
	}
	return "new"
}

func scanLead(scanner leadRowScanner) (*models.Leads, error) {
	lead := &models.Leads{}
	var description sql.NullString
	var phone sql.NullString
	var source sql.NullString
	var branchID sql.NullInt64
	var branchName sql.NullString
	var status sql.NullString
	var isArchived bool
	var archivedAt sql.NullTime
	var archivedBy sql.NullInt64
	var archiveReason sql.NullString

	if err := scanner.Scan(
		&lead.ID,
		&lead.Title,
		&description,
		&phone,
		&source,
		&lead.CreatedAt,
		&lead.OwnerID,
		&branchID,
		&branchName,
		&status,
		&isArchived,
		&archivedAt,
		&archivedBy,
		&archiveReason,
	); err != nil {
		return nil, err
	}

	lead.Description = stringFromNull(description)
	lead.Phone = stringFromNull(phone)
	lead.Source = stringFromNull(source)
	if branchID.Valid {
		v := int(branchID.Int64)
		lead.BranchID = &v
	}
	if branchName.Valid {
		lead.BranchName = branchName.String
	}
	lead.Status = normalizeLeadStatus(status)
	lead.IsArchived = isArchived
	if archivedAt.Valid {
		archived := archivedAt.Time
		lead.ArchivedAt = &archived
	}
	if archivedBy.Valid {
		by := int(archivedBy.Int64)
		lead.ArchivedBy = &by
	}
	lead.ArchiveReason = stringFromNull(archiveReason)
	return lead, nil
}

func leadArchiveWhere(scope ArchiveScope) string {
	switch scope {
	case ArchiveScopeArchivedOnly:
		return "is_archived = TRUE"
	case ArchiveScopeAll:
		return "1=1"
	default:
		return "is_archived = FALSE"
	}
}

// Создание лида с возвратом ID + created_at из БД
func (r *LeadRepository) Create(lead *models.Leads) (int64, error) {
	const query = `
		INSERT INTO leads (title, description, owner_id, branch_id, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`

	var id int64
	err := r.db.QueryRow(
		query,
		lead.Title,
		lead.Description,
		lead.OwnerID,
		lead.BranchID,
		lead.Status,
	).Scan(&id, &lead.CreatedAt)
	if err != nil {
		return 0, fmt.Errorf("create lead: %w", err)
	}
	return id, nil
}

// Обновление лида БЕЗ изменения created_at
func (r *LeadRepository) Update(lead *models.Leads) error {
	const query = `
		UPDATE leads
		SET title = $1,
		    description = $2,
		    owner_id = $3,
		    branch_id = $4,
		    status = $5
		WHERE id = $6
	`
	_, err := r.db.Exec(
		query,
		lead.Title,
		lead.Description,
		lead.OwnerID,
		lead.BranchID,
		lead.Status,
		lead.ID,
	)
	if err != nil {
		return fmt.Errorf("update lead: %w", err)
	}
	return nil
}

// GetByID: корректно обрабатывает отсутствие строки
func (r *LeadRepository) GetByID(id int) (*models.Leads, error) {
	return r.GetByIDWithArchiveScope(id, ArchiveScopeActiveOnly)
}

func (r *LeadRepository) GetByIDWithArchiveScope(id int, scope ArchiveScope) (*models.Leads, error) {
	const query = `
		SELECT l.id, l.title, l.description, l.phone, l.source, l.created_at, l.owner_id, l.branch_id, COALESCE(b.name,''), l.status, l.is_archived, l.archived_at, l.archived_by, l.archive_reason FROM leads l LEFT JOIN branches b ON b.id=l.branch_id
		WHERE l.id = $1 AND %s
	`
	row := r.db.QueryRow(fmt.Sprintf(query, leadArchiveWhere(scope)), id)
	lead, err := scanLead(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get lead by id: %w", err)
	}
	return lead, nil
}

func (r *LeadRepository) Delete(id int) error {
	const query = `DELETE FROM leads WHERE id=$1`
	_, err := r.db.Exec(query, id)
	return err
}

func (r *LeadRepository) Archive(id, archivedBy int, reason string) error {
	const query = `
		UPDATE leads
		SET is_archived = TRUE,
		    archived_at = NOW(),
		    archived_by = $2,
		    archive_reason = $3
		WHERE id = $1
	`
	_, err := r.db.Exec(query, id, archivedBy, reason)
	return err
}

func (r *LeadRepository) Unarchive(id int) error {
	const query = `
		UPDATE leads
		SET is_archived = FALSE,
		    archived_at = NULL,
		    archived_by = NULL,
		    archive_reason = NULL
		WHERE id = $1
	`
	_, err := r.db.Exec(query, id)
	return err
}

func (r *LeadRepository) CountLeads() (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM leads WHERE is_archived = FALSE`).Scan(&count)
	return count, err
}

func (r *LeadRepository) FilterLeads(status string, ownerID int, sortBy, order string, limit, offset int) ([]models.Leads, error) {
	if sortBy == "" {
		sortBy = "created_at"
	}
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	allowed := map[string]bool{"created_at": true, "owner_id": true, "status": true}
	if !allowed[sortBy] {
		sortBy = "created_at"
	}

	query := "SELECT l.id, l.title, l.description, l.phone, l.source, l.created_at, l.owner_id, l.branch_id, COALESCE(b.name,''), l.status, l.is_archived, l.archived_at, l.archived_by, l.archive_reason FROM leads l LEFT JOIN branches b ON b.id=l.branch_id WHERE l.is_archived = FALSE"
	args := []interface{}{}
	i := 1

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", i)
		args = append(args, status)
		i++
	}
	if ownerID > 0 {
		query += fmt.Sprintf(" AND owner_id = $%d", i)
		args = append(args, ownerID)
		i++
	}

	query += fmt.Sprintf(" ORDER BY %s %s LIMIT $%d OFFSET $%d", sortBy, order, i, i+1)
	args = append(args, limit, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Leads
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, nil
}

func (r *LeadRepository) ListAll(limit, offset int) ([]*models.Leads, error) {
	return r.ListAllWithFilterAndArchiveScope(limit, offset, LeadListFilter{}, ArchiveScopeActiveOnly)
}

func (r *LeadRepository) ListAllWithArchiveScope(limit, offset int, scope ArchiveScope) ([]*models.Leads, error) {
	return r.ListAllWithFilterAndArchiveScope(limit, offset, LeadListFilter{}, scope)
}

func (r *LeadRepository) ListAllWithFilterAndArchiveScope(limit, offset int, filter LeadListFilter, scope ArchiveScope) ([]*models.Leads, error) {
	const query = `
		SELECT l.id, l.title, l.description, l.phone, l.source, l.created_at, l.owner_id, l.branch_id, COALESCE(b.name,''), l.status, l.is_archived, l.archived_at, l.archived_by, l.archive_reason
		FROM leads l LEFT JOIN branches b ON b.id=l.branch_id
		WHERE %s%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`
	sortExpr, sortOrder := leadSortExpression(filter)
	extraWhere, args := buildLeadListWhere(filter, 1)
	args = append(args, limit, offset)
	rows, err := r.db.Query(
		fmt.Sprintf(
			query,
			leadArchiveWhere(scope),
			extraWhere,
			sortExpr,
			sortOrder,
			len(args)-1,
			len(args),
		),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.Leads
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

func (r *LeadRepository) ListPaginated(limit, offset int) ([]*models.Leads, error) {
	return r.ListAll(limit, offset)
}

// «Только мои» лиды
func (r *LeadRepository) ListByOwner(ownerID, limit, offset int) ([]*models.Leads, error) {
	return r.ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset, LeadListFilter{}, ArchiveScopeActiveOnly)
}

func (r *LeadRepository) ListByOwnerWithArchiveScope(ownerID, limit, offset int, scope ArchiveScope) ([]*models.Leads, error) {
	return r.ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset, LeadListFilter{}, scope)
}

func (r *LeadRepository) ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset int, filter LeadListFilter, scope ArchiveScope) ([]*models.Leads, error) {
	const query = `
		SELECT l.id, l.title, l.description, l.phone, l.source, l.created_at, l.owner_id, l.branch_id, COALESCE(b.name,''), l.status, l.is_archived, l.archived_at, l.archived_by, l.archive_reason
		FROM leads l LEFT JOIN branches b ON b.id=l.branch_id
		WHERE owner_id = $1 AND %s%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`
	sortExpr, sortOrder := leadSortExpression(filter)
	extraWhere, args := buildLeadListWhere(filter, 2)
	args = append([]interface{}{ownerID}, args...)
	args = append(args, limit, offset)
	rows, err := r.db.Query(
		fmt.Sprintf(
			query,
			leadArchiveWhere(scope),
			extraWhere,
			sortExpr,
			sortOrder,
			len(args)-1,
			len(args),
		),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.Leads
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

func buildLeadListWhere(filter LeadListFilter, startAt int) (string, []interface{}) {
	where := ""
	args := make([]interface{}, 0, 4)
	idx := startAt

	if filter.Status != "" {
		where += fmt.Sprintf(" AND COALESCE(status, 'new') = $%d", idx)
		args = append(args, filter.Status)
		idx++
	} else {
		statuses := leadStatusesFromGroup(filter.StatusGroup)
		if len(statuses) > 0 {
			where += fmt.Sprintf(" AND COALESCE(status, 'new') = ANY($%d)", idx)
			args = append(args, pq.Array(statuses))
			idx++
		}
	}
	if filter.Query != "" {
		likePattern := "%" + strings.ToLower(strings.TrimSpace(filter.Query)) + "%"
		where += fmt.Sprintf(` AND (
			LOWER(COALESCE(l.title::text, '')) LIKE $%d OR
			LOWER(COALESCE(l.description::text, '')) LIKE $%d OR
			LOWER(COALESCE(l.phone::text, '')) LIKE $%d
		)`, idx, idx, idx)
		args = append(args, likePattern)
		idx++
	}
	if filter.BranchID != nil {
		where += fmt.Sprintf(" AND l.branch_id = $%d", idx)
		args = append(args, *filter.BranchID)
		idx++
	}

	return where, args
}

func leadStatusesFromGroup(group string) []string {
	switch strings.ToLower(strings.TrimSpace(group)) {
	case "active":
		return []string{"new", "in_progress", "confirmed"}
	case "closed":
		return []string{"converted", "cancelled"}
	default:
		return nil
	}
}

func leadSortExpression(filter LeadListFilter) (string, string) {
	order := "DESC"
	if strings.EqualFold(filter.Order, "asc") {
		order = "ASC"
	}
	switch filter.SortBy {
	case "status":
		return "COALESCE(status, 'new')", order
	case "title":
		return "LOWER(COALESCE(title, ''))", order
	default:
		return "created_at", order
	}
}

func (r *LeadRepository) UpdateStatus(id int, status string) error {
	const q = `UPDATE leads SET status = $1 WHERE id = $2`
	_, err := r.db.Exec(q, status, id)
	return err
}

func (r *LeadRepository) UpdateOwner(id, ownerID int) error {
	const q = `UPDATE leads SET owner_id = $1 WHERE id = $2`
	_, err := r.db.Exec(q, ownerID, id)
	return err
}

// GetLeadsSummaryStats возвращает количество лидов по статусам и источникам (если они есть) за период.
func (r *LeadRepository) GetLeadsSummaryStats(ctx context.Context, from, to time.Time, ownerID *int, branchID *int) ([]models.LeadSummaryRow, error) {
	query := `SELECT COALESCE(status, 'new') AS status, '' AS source, COUNT(*) AS count FROM leads WHERE created_at BETWEEN $1 AND $2`
	args := []interface{}{from, to}
	idx := 3

	if ownerID != nil {
		query += fmt.Sprintf(" AND owner_id = $%d", idx)
		args = append(args, *ownerID)
		idx++
	}
	if branchID != nil {
		query += fmt.Sprintf(" AND branch_id = $%d", idx)
		args = append(args, *branchID)
	}

	query += " GROUP BY status ORDER BY status"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("leads summary stats: %w", err)
	}
	defer rows.Close()

	var result []models.LeadSummaryRow
	for rows.Next() {
		var row models.LeadSummaryRow
		if err := rows.Scan(&row.Status, &row.Source, &row.Count); err != nil {
			return nil, fmt.Errorf("scan leads summary row: %w", err)
		}
		result = append(result, row)
	}

	return result, nil
}

func (r *LeadRepository) ConvertToDeal(ctx context.Context, leadID int, deal *models.Deals, client *models.Client) (*models.Deals, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin convert lead tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var leadStatus sql.NullString
	if err = tx.QueryRow(`SELECT status FROM leads WHERE id = $1 FOR UPDATE`, leadID).Scan(&leadStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("lead not found")
		}
		return nil, fmt.Errorf("lock lead: %w", err)
	}

	loadClientType := func(clientID int) (string, error) {
		var clientType sql.NullString
		if err := tx.QueryRow(`SELECT client_type FROM clients WHERE id = $1`, clientID).Scan(&clientType); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return "", nil
			}
			return "", err
		}
		return stringFromNull(clientType), nil
	}

	loadExistingDealForUpdate := func(leadID int) (*models.Deals, error) {
		existing := &models.Deals{}
		var status sql.NullString
		err := tx.QueryRow(`
			SELECT d.id, d.lead_id, d.client_id, d.owner_id, d.amount, d.currency, d.status, d.created_at
			FROM deals d
			WHERE d.lead_id = $1
			ORDER BY d.created_at DESC
			LIMIT 1
			FOR UPDATE
		`, leadID).Scan(
			&existing.ID,
			&existing.LeadID,
			&existing.ClientID,
			&existing.OwnerID,
			&existing.Amount,
			&existing.Currency,
			&status,
			&existing.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		existing.Status = normalizeDealStatus(status)
		existing.ClientType, err = loadClientType(existing.ClientID)
		if err != nil {
			return nil, err
		}
		return existing, nil
	}

	existing, existingErr := loadExistingDealForUpdate(leadID)
	if existingErr == nil {
		if normalizeLeadStatus(leadStatus) != "converted" {
			if _, err = tx.Exec(`UPDATE leads SET status = 'converted' WHERE id = $1`, leadID); err != nil {
				return nil, fmt.Errorf("update lead status after existing deal: %w", err)
			}
		}
		if err = tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit existing deal conversion: %w", err)
		}
		return existing, ErrDealAlreadyExists
	}
	if !errors.Is(existingErr, sql.ErrNoRows) {
		return nil, fmt.Errorf("check existing deal: %w", existingErr)
	}

	leadStatusValue := normalizeLeadStatus(leadStatus)
	if leadStatusValue != "confirmed" {
		return nil, errors.New("lead is not in a convertible status")
	}

	if client == nil {
		return nil, errors.New("client data is required")
	}
	if client.ID == 0 {
		return nil, errors.New("client data is required")
	}
	var storedClientType string
	if err = tx.QueryRow(`SELECT id, client_type FROM clients WHERE id = $1 FOR UPDATE`, client.ID).Scan(&deal.ClientID, &storedClientType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrClientNotFound
		}
		return nil, fmt.Errorf("lookup client: %w", err)
	}
	if strings.TrimSpace(client.ClientType) == "" {
		return nil, errors.New("client_type is required")
	}
	if strings.ToLower(strings.TrimSpace(client.ClientType)) != strings.ToLower(strings.TrimSpace(storedClientType)) {
		return nil, errors.New("client_type does not match stored client type")
	}
	deal.ClientType = strings.ToLower(strings.TrimSpace(storedClientType))

	err = tx.QueryRow(`
		INSERT INTO deals (lead_id, client_id, owner_id, branch_id, amount, currency, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (lead_id) DO NOTHING
		RETURNING id
	`,
		deal.LeadID,
		deal.ClientID,
		deal.OwnerID,
		deal.BranchID,
		deal.Amount,
		deal.Currency,
		deal.Status,
		deal.CreatedAt,
	).Scan(&deal.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			existing, err := loadExistingDealForUpdate(leadID)
			if err != nil {
				return nil, fmt.Errorf("fetch existing deal after conflict: %w", err)
			}
			if _, err = tx.Exec(`UPDATE leads SET status = 'converted' WHERE id = $1`, leadID); err != nil {
				return nil, fmt.Errorf("update lead status after conflict: %w", err)
			}
			if err = tx.Commit(); err != nil {
				return nil, fmt.Errorf("commit conflict conversion: %w", err)
			}
			return existing, ErrDealAlreadyExists
		}
		return nil, fmt.Errorf("insert deal: %w", err)
	}

	if _, err = tx.Exec(`UPDATE leads SET status = 'converted' WHERE id = $1`, leadID); err != nil {
		return nil, fmt.Errorf("update lead status: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit convert lead tx: %w", err)
	}

	return deal, nil
}
