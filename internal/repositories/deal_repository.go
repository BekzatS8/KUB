package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"turcompany/internal/models"
)

type DealRepository struct {
	db *sql.DB
}

type DealListFilter struct {
	ClientID    int
	ClientType  string
	Query       string
	Status      string
	StatusGroup string
	AmountMin   *float64
	AmountMax   *float64
	Currency    string
	SortBy      string
	Order       string
	BranchID    *int
}

func NewDealRepository(db *sql.DB) *DealRepository {
	return &DealRepository{db: db}
}

func normalizeDealStatus(status sql.NullString) string {
	if status.Valid && status.String != "" {
		return status.String
	}
	return "new"
}

func dealArchiveWhere(scope ArchiveScope, alias string) string {
	column := "is_archived"
	if alias != "" {
		column = alias + ".is_archived"
	}
	switch scope {
	case ArchiveScopeArchivedOnly:
		return column + " = TRUE"
	case ArchiveScopeAll:
		return "1=1"
	default:
		return column + " = FALSE"
	}
}

// Создание сделки — возвращает ID новой записи
func (r *DealRepository) Create(deal *models.Deals) (int64, error) {
	query := `
		INSERT INTO deals (lead_id, client_id, owner_id, branch_id, amount, currency, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`
	var id int64
	err := r.db.QueryRow(
		query,
		deal.LeadID,    // $1
		deal.ClientID,  // $2
		deal.OwnerID,   // $3
		deal.BranchID,  // $4
		deal.Amount,    // $5
		deal.Currency,  // $6
		deal.Status,    // $7
		deal.CreatedAt, // $8
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("создание сделки: %w", err)
	}
	return id, nil
}

// Получение сделки по lead_id (последняя по времени)
func (r *DealRepository) GetByLeadID(leadID int) (*models.Deals, error) {
	return r.GetByLeadIDWithArchiveScope(leadID, ArchiveScopeActiveOnly)
}

func (r *DealRepository) GetByLeadIDWithArchiveScope(leadID int, scope ArchiveScope) (*models.Deals, error) {
	query := `
		SELECT d.id, d.lead_id, d.client_id, COALESCE(c.client_type, ''), d.owner_id, d.branch_id, COALESCE(b.name,''), d.amount, d.currency, d.status, d.created_at, d.is_archived, d.archived_at, d.archived_by, d.archive_reason
		FROM deals d
		LEFT JOIN clients c ON c.id = d.client_id
		LEFT JOIN branches b ON b.id = d.branch_id
		WHERE d.lead_id = $1 AND %s
		ORDER BY d.created_at DESC
		LIMIT 1
	`

	deal := &models.Deals{}
	var status sql.NullString
	var branchID sql.NullInt64
	var branchName sql.NullString

	var isArchived bool
	var archivedAt sql.NullTime
	var archivedBy sql.NullInt64
	var archiveReason sql.NullString

	err := r.db.QueryRow(fmt.Sprintf(query, dealArchiveWhere(scope, "d")), leadID).Scan(
		&deal.ID,
		&deal.LeadID,
		&deal.ClientID,
		&deal.ClientType,
		&deal.OwnerID,
		&branchID,
		&branchName,
		&deal.Amount,
		&deal.Currency,
		&status,
		&deal.CreatedAt,
		&isArchived,
		&archivedAt,
		&archivedBy,
		&archiveReason,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("получение сделки по lead_id: %w", err)
	}

	deal.Status = normalizeDealStatus(status)
	if branchID.Valid {
		v := int(branchID.Int64)
		deal.BranchID = &v
	}
	if branchName.Valid {
		deal.BranchName = branchName.String
	}
	deal.IsArchived = isArchived
	if archivedAt.Valid {
		archived := archivedAt.Time
		deal.ArchivedAt = &archived
	}
	if archivedBy.Valid {
		by := int(archivedBy.Int64)
		deal.ArchivedBy = &by
	}
	deal.ArchiveReason = stringFromNull(archiveReason)
	return deal, nil
}

func (r *DealRepository) Update(deal *models.Deals) error {
	query := `
		UPDATE deals
		SET lead_id=$1, client_id=$2, owner_id=$3, branch_id=$4, amount=$5, currency=$6, status=$7
		WHERE id=$8
	`
	_, err := r.db.Exec(query,
		deal.LeadID,   // $1
		deal.ClientID, // $2
		deal.OwnerID,  // $3
		deal.BranchID, // $4
		deal.Amount,   // $5
		deal.Currency, // $6
		deal.Status,   // $7
		deal.ID,       // $8
	)

	if err != nil {
		return fmt.Errorf("обновление сделки: %w", err)
	}
	return nil
}

// Получение по ID
func (r *DealRepository) GetByID(id int) (*models.Deals, error) {
	return r.GetByIDWithArchiveScope(id, ArchiveScopeActiveOnly)
}

func (r *DealRepository) GetByIDWithArchiveScope(id int, scope ArchiveScope) (*models.Deals, error) {
	query := `
		SELECT d.id, d.lead_id, d.client_id, COALESCE(c.client_type, ''), d.owner_id, d.branch_id, COALESCE(b.name,''), d.amount, d.currency, d.status, d.created_at, d.is_archived, d.archived_at, d.archived_by, d.archive_reason
		FROM deals d
		LEFT JOIN clients c ON c.id = d.client_id
		LEFT JOIN branches b ON b.id = d.branch_id
		WHERE d.id=$1 AND %s
	`

	deal := &models.Deals{}
	var status sql.NullString
	var branchID sql.NullInt64
	var branchName sql.NullString

	var isArchived bool
	var archivedAt sql.NullTime
	var archivedBy sql.NullInt64
	var archiveReason sql.NullString

	err := r.db.QueryRow(fmt.Sprintf(query, dealArchiveWhere(scope, "d")), id).Scan(
		&deal.ID,
		&deal.LeadID,
		&deal.ClientID,
		&deal.ClientType,
		&deal.OwnerID,
		&branchID,
		&branchName,
		&deal.Amount,
		&deal.Currency,
		&status,
		&deal.CreatedAt,
		&isArchived,
		&archivedAt,
		&archivedBy,
		&archiveReason,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("получение сделки по id: %w", err)
	}

	deal.Status = normalizeDealStatus(status)
	if branchID.Valid {
		v := int(branchID.Int64)
		deal.BranchID = &v
	}
	if branchName.Valid {
		deal.BranchName = branchName.String
	}
	deal.IsArchived = isArchived
	if archivedAt.Valid {
		archived := archivedAt.Time
		deal.ArchivedAt = &archived
	}
	if archivedBy.Valid {
		by := int(archivedBy.Int64)
		deal.ArchivedBy = &by
	}
	deal.ArchiveReason = stringFromNull(archiveReason)
	return deal, nil
}

// Удаление по ID
func (r *DealRepository) Delete(id int) error {
	query := `DELETE FROM deals WHERE id=$1`
	result, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("удаление сделки: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("проверка удаления: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("сделка с id=%d не найдена", id)
	}
	return nil
}

func (r *DealRepository) Archive(id, archivedBy int, reason string) error {
	query := `
		UPDATE deals
		SET is_archived = TRUE,
		    archived_at = NOW(),
		    archived_by = $2,
		    archive_reason = $3
		WHERE id=$1
	`
	_, err := r.db.Exec(query, id, archivedBy, reason)
	return err
}

func (r *DealRepository) Unarchive(id int) error {
	query := `
		UPDATE deals
		SET is_archived = FALSE,
		    archived_at = NULL,
		    archived_by = NULL,
		    archive_reason = NULL
		WHERE id=$1
	`
	_, err := r.db.Exec(query, id)
	return err
}

// Подсчёт сделок
func (r *DealRepository) CountDeals() (int, error) {
	var count int
	query := "SELECT COUNT(*) FROM deals WHERE is_archived = FALSE"
	err := r.db.QueryRow(query).Scan(&count)
	return count, err
}

// Фильтрация
func (r *DealRepository) FilterDeals(status, fromDate, toDate, currency, sortBy, order string, amountMin, amountMax float64, limit, offset int) ([]models.Deals, error) {
	if sortBy == "" {
		sortBy = "created_at"
	}
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	allowedSortFields := map[string]string{
		"created_at": "d.created_at",
		"amount":     "d.amount",
		"status":     "d.status",
		"currency":   "d.currency",
	}
	sortExpr, ok := allowedSortFields[sortBy]
	if !ok {
		sortExpr = "d.created_at"
	}

	query := "SELECT d.id, d.lead_id, d.client_id, COALESCE(c.client_type, ''), d.owner_id, d.branch_id, COALESCE(b.name,''), d.amount, d.currency, d.status, d.created_at, d.is_archived, d.archived_at, d.archived_by, d.archive_reason FROM deals d LEFT JOIN clients c ON c.id = d.client_id LEFT JOIN branches b ON b.id = d.branch_id WHERE d.is_archived = FALSE"
	args := []interface{}{}
	i := 1

	if status != "" {
		query += fmt.Sprintf(" AND d.status = $%d", i)
		args = append(args, status)
		i++
	}
	if fromDate != "" {
		query += fmt.Sprintf(" AND d.created_at >= $%d", i)
		args = append(args, fromDate)
		i++
	}
	if toDate != "" {
		query += fmt.Sprintf(" AND d.created_at <= $%d", i)
		args = append(args, toDate)
		i++
	}
	if currency != "" {
		query += fmt.Sprintf(" AND d.currency = $%d", i)
		args = append(args, currency)
		i++
	}
	if amountMin > 0 {
		query += fmt.Sprintf(" AND d.amount >= $%d", i)
		args = append(args, amountMin)
		i++
	}
	if amountMax > 0 {
		query += fmt.Sprintf(" AND d.amount <= $%d", i)
		args = append(args, amountMax)
		i++
	}

	query += fmt.Sprintf(" ORDER BY %s %s LIMIT $%d OFFSET $%d", sortExpr, order, i, i+1)
	args = append(args, limit, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deals []models.Deals
	for rows.Next() {
		var deal models.Deals
		var status sql.NullString
		var branchID sql.NullInt64
		var branchName sql.NullString
		var isArchived bool
		var archivedAt sql.NullTime
		var archivedBy sql.NullInt64
		var archiveReason sql.NullString

		if err := rows.Scan(
			&deal.ID,
			&deal.LeadID,
			&deal.ClientID,
			&deal.ClientType,
			&deal.OwnerID,
			&branchID,
			&branchName,
			&deal.Amount,
			&deal.Currency,
			&status,
			&deal.CreatedAt,
			&isArchived,
			&archivedAt,
			&archivedBy,
			&archiveReason,
		); err != nil {
			return nil, err
		}

		deal.Status = normalizeDealStatus(status)
		if branchID.Valid {
			v := int(branchID.Int64)
			deal.BranchID = &v
		}
		if branchName.Valid {
			deal.BranchName = branchName.String
		}
		deal.IsArchived = isArchived
		if archivedAt.Valid {
			archived := archivedAt.Time
			deal.ArchivedAt = &archived
		}
		if archivedBy.Valid {
			by := int(archivedBy.Int64)
			deal.ArchivedBy = &by
		}
		deal.ArchiveReason = stringFromNull(archiveReason)
		deals = append(deals, deal)
	}
	return deals, nil
}

func (r *DealRepository) ListAll(limit, offset int) ([]*models.Deals, error) {
	return r.ListAllWithFilterAndArchiveScope(limit, offset, DealListFilter{}, ArchiveScopeActiveOnly)
}

func (r *DealRepository) ListAllWithArchiveScope(limit, offset int, scope ArchiveScope) ([]*models.Deals, error) {
	return r.ListAllWithFilterAndArchiveScope(limit, offset, DealListFilter{}, scope)
}

func (r *DealRepository) ListAllWithFilterAndArchiveScope(limit, offset int, filter DealListFilter, scope ArchiveScope) ([]*models.Deals, error) {
	query := `
		SELECT d.id, d.lead_id, d.client_id, COALESCE(c.client_type, ''), d.owner_id, d.branch_id, COALESCE(b.name,''), d.amount, d.currency, d.status, d.created_at, d.is_archived, d.archived_at, d.archived_by, d.archive_reason
		FROM deals d
		LEFT JOIN clients c ON c.id = d.client_id
		LEFT JOIN branches b ON b.id = d.branch_id
		WHERE %s%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`

	sortExpr, sortOrder := dealSortExpression(filter)
	extraWhere, args := buildDealListWhere(filter, 1)
	args = append(args, limit, offset)
	rows, err := r.db.Query(
		fmt.Sprintf(
			query,
			dealArchiveWhere(scope, "d"),
			extraWhere,
			sortExpr,
			sortOrder,
			len(args)-1,
			len(args),
		),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса: %w", err)
	}
	defer rows.Close()

	var deals []*models.Deals
	for rows.Next() {
		var d models.Deals
		var status sql.NullString
		var branchID sql.NullInt64
		var branchName sql.NullString
		var isArchived bool
		var archivedAt sql.NullTime
		var archivedBy sql.NullInt64
		var archiveReason sql.NullString

		if err := rows.Scan(
			&d.ID,
			&d.LeadID,
			&d.ClientID,
			&d.ClientType,
			&d.OwnerID,
			&branchID,
			&branchName,
			&d.Amount,
			&d.Currency,
			&status,
			&d.CreatedAt,
			&isArchived,
			&archivedAt,
			&archivedBy,
			&archiveReason,
		); err != nil {
			return nil, fmt.Errorf("ошибка чтения: %w", err)
		}

		d.Status = normalizeDealStatus(status)
		if branchID.Valid {
			v := int(branchID.Int64)
			d.BranchID = &v
		}
		if branchName.Valid {
			d.BranchName = branchName.String
		}
		d.IsArchived = isArchived
		if archivedAt.Valid {
			archived := archivedAt.Time
			d.ArchivedAt = &archived
		}
		if archivedBy.Valid {
			by := int(archivedBy.Int64)
			d.ArchivedBy = &by
		}
		d.ArchiveReason = stringFromNull(archiveReason)
		deals = append(deals, &d)
	}
	return deals, nil
}

func (r *DealRepository) ListPaginated(limit, offset int) ([]*models.Deals, error) {
	return r.ListAll(limit, offset)
}

// Только сделки конкретного владельца
func (r *DealRepository) ListByOwner(ownerID, limit, offset int) ([]*models.Deals, error) {
	return r.ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset, DealListFilter{}, ArchiveScopeActiveOnly)
}

func (r *DealRepository) ListByOwnerWithArchiveScope(ownerID, limit, offset int, scope ArchiveScope) ([]*models.Deals, error) {
	return r.ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset, DealListFilter{}, scope)
}

func (r *DealRepository) ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset int, filter DealListFilter, scope ArchiveScope) ([]*models.Deals, error) {
	query := `
		SELECT d.id, d.lead_id, d.client_id, COALESCE(c.client_type, ''), d.owner_id, d.branch_id, COALESCE(b.name,''), d.amount, d.currency, d.status, d.created_at, d.is_archived, d.archived_at, d.archived_by, d.archive_reason
		FROM deals d
		LEFT JOIN clients c ON c.id = d.client_id
		LEFT JOIN branches b ON b.id = d.branch_id
		WHERE d.owner_id = $1 AND %s%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`

	sortExpr, sortOrder := dealSortExpression(filter)
	extraWhere, args := buildDealListWhere(filter, 2)
	args = append([]interface{}{ownerID}, args...)
	args = append(args, limit, offset)
	rows, err := r.db.Query(
		fmt.Sprintf(
			query,
			dealArchiveWhere(scope, "d"),
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

	var deals []*models.Deals
	for rows.Next() {
		var d models.Deals
		var status sql.NullString
		var branchID sql.NullInt64
		var branchName sql.NullString
		var isArchived bool
		var archivedAt sql.NullTime
		var archivedBy sql.NullInt64
		var archiveReason sql.NullString

		if err := rows.Scan(
			&d.ID,
			&d.LeadID,
			&d.ClientID,
			&d.ClientType,
			&d.OwnerID,
			&branchID,
			&branchName,
			&d.Amount,
			&d.Currency,
			&status,
			&d.CreatedAt,
			&isArchived,
			&archivedAt,
			&archivedBy,
			&archiveReason,
		); err != nil {
			return nil, err
		}

		d.Status = normalizeDealStatus(status)
		if branchID.Valid {
			v := int(branchID.Int64)
			d.BranchID = &v
		}
		if branchName.Valid {
			d.BranchName = branchName.String
		}
		d.IsArchived = isArchived
		if archivedAt.Valid {
			archived := archivedAt.Time
			d.ArchivedAt = &archived
		}
		if archivedBy.Valid {
			by := int(archivedBy.Int64)
			d.ArchivedBy = &by
		}
		d.ArchiveReason = stringFromNull(archiveReason)
		deals = append(deals, &d)
	}
	return deals, nil
}

func (r *DealRepository) CountAllWithFilterAndArchiveScope(filter DealListFilter, scope ArchiveScope) (int, error) {
	extraWhere, args := buildDealListWhere(filter, 1)
	query := fmt.Sprintf(`SELECT COUNT(1) FROM deals d LEFT JOIN clients c ON c.id = d.client_id WHERE %s%s`, dealArchiveWhere(scope, "d"), extraWhere)
	var total int
	if err := r.db.QueryRow(query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (r *DealRepository) CountByOwnerWithFilterAndArchiveScope(ownerID int, filter DealListFilter, scope ArchiveScope) (int, error) {
	extraWhere, args := buildDealListWhere(filter, 2)
	args = append([]interface{}{ownerID}, args...)
	query := fmt.Sprintf(`SELECT COUNT(1) FROM deals d LEFT JOIN clients c ON c.id = d.client_id WHERE d.owner_id = $1 AND %s%s`, dealArchiveWhere(scope, "d"), extraWhere)
	var total int
	if err := r.db.QueryRow(query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func buildDealListWhere(filter DealListFilter, startAt int) (string, []interface{}) {
	where := ""
	args := make([]interface{}, 0, 10)
	idx := startAt

	if filter.ClientID > 0 {
		where += fmt.Sprintf(" AND d.client_id = $%d", idx)
		args = append(args, filter.ClientID)
		idx++
	}
	if filter.ClientType != "" {
		where += fmt.Sprintf(" AND c.client_type = $%d", idx)
		args = append(args, filter.ClientType)
		idx++
	}
	if filter.Status != "" {
		where += fmt.Sprintf(" AND COALESCE(d.status, 'new') = $%d", idx)
		args = append(args, filter.Status)
		idx++
	} else {
		statuses := dealStatusesFromGroup(filter.StatusGroup)
		if len(statuses) > 0 {
			where += fmt.Sprintf(" AND COALESCE(d.status, 'new') = ANY($%d)", idx)
			args = append(args, pq.Array(statuses))
			idx++
		}
	}
	if filter.AmountMin != nil {
		where += fmt.Sprintf(" AND d.amount >= $%d", idx)
		args = append(args, *filter.AmountMin)
		idx++
	}
	if filter.AmountMax != nil {
		where += fmt.Sprintf(" AND d.amount <= $%d", idx)
		args = append(args, *filter.AmountMax)
		idx++
	}
	if filter.Currency != "" {
		where += fmt.Sprintf(" AND UPPER(COALESCE(d.currency, '')) = $%d", idx)
		args = append(args, strings.ToUpper(filter.Currency))
		idx++
	}
	if filter.BranchID != nil {
		where += fmt.Sprintf(" AND d.branch_id = $%d", idx)
		args = append(args, *filter.BranchID)
		idx++
	}
	if filter.Query != "" {
		likePattern := "%" + strings.ToLower(filter.Query) + "%"
		where += fmt.Sprintf(` AND (
			LOWER(COALESCE(c.display_name, c.name, '')) LIKE $%d OR
			LOWER(COALESCE(c.bin_iin, '')) LIKE $%d OR
			LOWER(COALESCE(c.primary_phone, c.phone, '')) LIKE $%d OR
			LOWER(COALESCE(c.primary_email, c.email, '')) LIKE $%d OR
			CAST(d.amount AS TEXT) LIKE $%d OR
			LOWER(COALESCE(d.currency, '')) LIKE $%d
		)`, idx, idx, idx, idx, idx, idx)
		args = append(args, likePattern)
	}

	return where, args
}

func dealStatusesFromGroup(group string) []string {
	switch strings.ToLower(strings.TrimSpace(group)) {
	case "active":
		return []string{"new", "in_progress", "negotiation"}
	case "completed":
		return []string{"won"}
	case "closed":
		return []string{"lost", "cancelled"}
	default:
		return nil
	}
}

func dealSortExpression(filter DealListFilter) (string, string) {
	order := "DESC"
	if strings.EqualFold(filter.Order, "asc") {
		order = "ASC"
	}
	switch filter.SortBy {
	case "amount":
		return "d.amount", order
	case "status":
		return "COALESCE(d.status, 'new')", order
	case "client_name":
		return "LOWER(COALESCE(c.display_name, c.name, ''))", order
	default:
		return "d.created_at", order
	}
}

func (r *DealRepository) UpdateStatus(id int, status string) error {
	const q = `UPDATE deals SET status = $1 WHERE id = $2`
	_, err := r.db.Exec(q, status, id)
	return err
}

// GetLatestByClientID возвращает последнюю сделку по client_id
func (r *DealRepository) GetLatestByClientID(clientID int) (*models.Deals, error) {
	query := `
		SELECT d.id, d.lead_id, d.client_id, COALESCE(c.client_type, ''), d.owner_id, d.branch_id, COALESCE(b.name,''), d.amount, d.currency, d.status, d.created_at, d.is_archived, d.archived_at, d.archived_by, d.archive_reason
		FROM deals d
		LEFT JOIN clients c ON c.id = d.client_id
		LEFT JOIN branches b ON b.id = d.branch_id
		WHERE d.client_id = $1 AND d.is_archived = FALSE
		ORDER BY d.created_at DESC
		LIMIT 1
	`

	deal := &models.Deals{}
	var status sql.NullString
	var branchID sql.NullInt64
	var branchName sql.NullString
	var isArchived bool
	var archivedAt sql.NullTime
	var archivedBy sql.NullInt64
	var archiveReason sql.NullString

	err := r.db.QueryRow(query, clientID).Scan(
		&deal.ID,
		&deal.LeadID,
		&deal.ClientID,
		&deal.ClientType,
		&deal.OwnerID,
		&branchID,
		&branchName,
		&deal.Amount,
		&deal.Currency,
		&status,
		&deal.CreatedAt,
		&isArchived,
		&archivedAt,
		&archivedBy,
		&archiveReason,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get deal by client_id: %w", err)
	}

	deal.Status = normalizeDealStatus(status)
	if branchID.Valid {
		v := int(branchID.Int64)
		deal.BranchID = &v
	}
	if branchName.Valid {
		deal.BranchName = branchName.String
	}
	deal.IsArchived = isArchived
	if archivedAt.Valid {
		archived := archivedAt.Time
		deal.ArchivedAt = &archived
	}
	if archivedBy.Valid {
		by := int(archivedBy.Int64)
		deal.ArchivedBy = &by
	}
	deal.ArchiveReason = stringFromNull(archiveReason)
	return deal, nil
}

// GetLatestByClientRef возвращает последнюю сделку по точной typed ссылке клиента.
func (r *DealRepository) GetLatestByClientRef(clientID int, clientType string) (*models.Deals, error) {
	query := `
		SELECT d.id, d.lead_id, d.client_id, COALESCE(c.client_type, ''), d.owner_id, d.branch_id, COALESCE(b.name,''), d.amount, d.currency, d.status, d.created_at, d.is_archived, d.archived_at, d.archived_by, d.archive_reason
		FROM deals d
		JOIN clients c ON c.id = d.client_id
		WHERE d.client_id = $1 AND c.client_type = $2 AND d.is_archived = FALSE
		ORDER BY d.created_at DESC
		LIMIT 1
	`

	deal := &models.Deals{}
	var status sql.NullString
	var branchID sql.NullInt64
	var branchName sql.NullString
	var isArchived bool
	var archivedAt sql.NullTime
	var archivedBy sql.NullInt64
	var archiveReason sql.NullString

	err := r.db.QueryRow(query, clientID, clientType).Scan(
		&deal.ID,
		&deal.LeadID,
		&deal.ClientID,
		&deal.ClientType,
		&deal.OwnerID,
		&branchID,
		&branchName,
		&deal.Amount,
		&deal.Currency,
		&status,
		&deal.CreatedAt,
		&isArchived,
		&archivedAt,
		&archivedBy,
		&archiveReason,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest deal by typed client ref: %w", err)
	}
	deal.Status = normalizeDealStatus(status)
	if branchID.Valid {
		v := int(branchID.Int64)
		deal.BranchID = &v
	}
	if branchName.Valid {
		deal.BranchName = branchName.String
	}
	deal.IsArchived = isArchived
	if archivedAt.Valid {
		archived := archivedAt.Time
		deal.ArchivedAt = &archived
	}
	if archivedBy.Valid {
		by := int(archivedBy.Int64)
		deal.ArchivedBy = &by
	}
	deal.ArchiveReason = stringFromNull(archiveReason)
	return deal, nil
}

// GetDealsFunnelStats возвращает количество сделок по статусам за указанный период.
func (r *DealRepository) GetDealsFunnelStats(ctx context.Context, from, to time.Time, ownerID *int, branchID *int) ([]models.FunnelRow, error) {
	query := `SELECT COALESCE(status, 'new') AS status, COUNT(*) AS count FROM deals WHERE created_at BETWEEN $1 AND $2`
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
		return nil, fmt.Errorf("deals funnel stats: %w", err)
	}
	defer rows.Close()

	var result []models.FunnelRow
	for rows.Next() {
		var row models.FunnelRow
		if err := rows.Scan(&row.Status, &row.Count); err != nil {
			return nil, fmt.Errorf("scan deals funnel row: %w", err)
		}
		result = append(result, row)
	}

	return result, nil
}

// GetDealsRevenueStats возвращает суммы выигранных сделок по месяцам за период.
func (r *DealRepository) GetDealsRevenueStats(ctx context.Context, from, to time.Time, ownerID *int, branchID *int) ([]models.RevenueRow, error) {
	query := `
		SELECT
			TO_CHAR(date_trunc('month', created_at), 'YYYY-MM') AS period,
			SUM(amount) AS total_amount,
			currency
		FROM deals
		WHERE status = 'won' AND created_at BETWEEN $1 AND $2`
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

	query += " GROUP BY period, currency ORDER BY period"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("deals revenue stats: %w", err)
	}
	defer rows.Close()

	var result []models.RevenueRow
	for rows.Next() {
		var row models.RevenueRow
		if err := rows.Scan(&row.Period, &row.TotalAmount, &row.Currency); err != nil {
			return nil, fmt.Errorf("scan deals revenue row: %w", err)
		}
		result = append(result, row)
	}

	return result, nil
}

// GetTopClientsByRevenue возвращает топ клиентов по сумме выигранных сделок.
func (r *DealRepository) GetTopClientsByRevenue(ctx context.Context, from, to time.Time, ownerID *int, branchID *int, limit int) ([]models.TopClientRow, error) {
	query := `
		SELECT
			d.client_id,
			c.client_type,
			COALESCE(NULLIF(c.display_name, ''), c.name) AS client_name,
			SUM(d.amount) AS total_amount,
			d.currency
		FROM deals d
		JOIN clients c ON c.id = d.client_id
		WHERE d.status = 'won' AND d.created_at BETWEEN $1 AND $2`
	args := []interface{}{from, to}
	idx := 3

	if ownerID != nil {
		query += fmt.Sprintf(" AND d.owner_id = $%d", idx)
		args = append(args, *ownerID)
		idx++
	}
	if branchID != nil {
		query += fmt.Sprintf(" AND d.branch_id = $%d", idx)
		args = append(args, *branchID)
	}

	query += " GROUP BY d.client_id, c.client_type, COALESCE(NULLIF(c.display_name, ''), c.name), d.currency ORDER BY total_amount DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", len(args)+1)
		args = append(args, limit)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("top clients by revenue: %w", err)
	}
	defer rows.Close()

	var result []models.TopClientRow
	for rows.Next() {
		var row models.TopClientRow
		if err := rows.Scan(&row.ClientID, &row.ClientType, &row.ClientName, &row.TotalAmount, &row.Currency); err != nil {
			return nil, fmt.Errorf("scan top client row: %w", err)
		}
		result = append(result, row)
	}

	return result, nil
}
