package repositories

import (
	"database/sql"
	"fmt"
	"turcompany/internal/models"
)

type DealRepository struct {
	db *sql.DB
}

func NewDealRepository(db *sql.DB) *DealRepository {
	return &DealRepository{db: db}
}

// ✔ Возвращает ID новой сделки
func (r *DealRepository) Create(deal *models.Deals) (int64, error) {
	query := `
        INSERT INTO deals (lead_id, amount, currency, status, created_at) 
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id
    `
	var id int64
	err := r.db.QueryRow(
		query,
		deal.LeadID,
		deal.Amount,
		deal.Currency,
		deal.Status,
		deal.CreatedAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("создание сделки: %w", err)
	}
	return id, nil
}

// ✔ Получение сделки по lead_id (нужен для document/lead service)
func (r *DealRepository) GetByLeadID(leadID int) (*models.Deals, error) {
	query := `
        SELECT id, lead_id, amount, currency, status, created_at 
        FROM deals 
        WHERE lead_id = $1 
        ORDER BY created_at DESC 
        LIMIT 1
    `
	deal := &models.Deals{}
	err := r.db.QueryRow(query, leadID).Scan(
		&deal.ID,
		&deal.LeadID,
		&deal.Amount,
		&deal.Currency,
		&deal.Status,
		&deal.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("получение сделки по lead_id: %w", err)
	}
	return deal, nil
}

// ✔ Обновление
func (r *DealRepository) Update(deal *models.Deals) error {
	query := `
        UPDATE deals 
        SET lead_id=$1, amount=$2, currency=$3, status=$4 
        WHERE id=$5
    `
	_, err := r.db.Exec(query, deal.LeadID, deal.Amount, deal.Currency, deal.Status, deal.ID)
	if err != nil {
		return fmt.Errorf("обновление сделки: %w", err)
	}
	return nil
}

// ✔ Поиск по ID (тип int!)
func (r *DealRepository) GetByID(id int) (*models.Deals, error) {
	query := `
        SELECT id, lead_id, amount, currency, status, created_at 
        FROM deals 
        WHERE id=$1
    `
	deal := &models.Deals{}
	err := r.db.QueryRow(query, id).Scan(
		&deal.ID,
		&deal.LeadID,
		&deal.Amount,
		&deal.Currency,
		&deal.Status,
		&deal.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("получение сделки по id: %w", err)
	}
	return deal, nil
}

// ✔ Удаление по int ID
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

// ✔ Подсчёт сделок
func (r *DealRepository) CountDeals() (int, error) {
	var count int
	query := "SELECT COUNT(*) FROM deals"
	err := r.db.QueryRow(query).Scan(&count)
	return count, err
}

func (r *DealRepository) FilterDeals(status, fromDate, toDate, currency, sortBy, order string, amountMin, amountMax float64, limit, offset int) ([]models.Deals, error) {
	if sortBy == "" {
		sortBy = "created_at"
	}
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	allowedSortFields := map[string]bool{
		"created_at": true,
		"amount":     true,
		"status":     true,
		"currency":   true,
	}
	if !allowedSortFields[sortBy] {
		sortBy = "created_at"
	}

	query := "SELECT id, lead_id, amount, currency, status, created_at FROM deals WHERE 1=1"
	args := []interface{}{}
	i := 1

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", i)
		args = append(args, status)
		i++
	}
	if fromDate != "" {
		query += fmt.Sprintf(" AND created_at >= $%d", i)
		args = append(args, fromDate)
		i++
	}
	if toDate != "" {
		query += fmt.Sprintf(" AND created_at <= $%d", i)
		args = append(args, toDate)
		i++
	}
	if currency != "" {
		query += fmt.Sprintf(" AND currency = $%d", i)
		args = append(args, currency)
		i++
	}
	if amountMin > 0 {
		query += fmt.Sprintf(" AND amount::float >= $%d", i)
		args = append(args, amountMin)
		i++
	}
	if amountMax > 0 {
		query += fmt.Sprintf(" AND amount::float <= $%d", i)
		args = append(args, amountMax)
		i++
	}

	query += fmt.Sprintf(" ORDER BY %s %s LIMIT $%d OFFSET $%d", sortBy, order, i, i+1)
	args = append(args, limit, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deals []models.Deals
	for rows.Next() {
		var deal models.Deals
		if err := rows.Scan(&deal.ID, &deal.LeadID, &deal.Amount, &deal.Currency, &deal.Status, &deal.CreatedAt); err != nil {
			return nil, err
		}
		deals = append(deals, deal)
	}
	return deals, nil
}

func (r *DealRepository) ListPaginated(limit, offset int) ([]*models.Deals, error) {
	query := `SELECT id, lead_id, amount, currency, status, created_at 
	          FROM deals 
	          ORDER BY created_at DESC 
	          LIMIT $1 OFFSET $2`

	rows, err := r.db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса: %w", err)
	}
	defer rows.Close()

	var deals []*models.Deals
	for rows.Next() {
		var deal models.Deals
		if err := rows.Scan(&deal.ID, &deal.LeadID, &deal.Amount, &deal.Currency, &deal.Status, &deal.CreatedAt); err != nil {
			return nil, fmt.Errorf("ошибка чтения: %w", err)
		}
		deals = append(deals, &deal)
	}
	return deals, nil
}
