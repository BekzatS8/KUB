package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"turcompany/internal/models"
)

type LeadRepository struct {
	db *sql.DB
}

func NewLeadRepository(db *sql.DB) *LeadRepository {
	if db == nil {
		log.Fatalf("received nil database connection")
	}
	return &LeadRepository{db: db}
}

// Создание лида с возвратом ID + created_at из БД
func (r *LeadRepository) Create(lead *models.Leads) (int64, error) {
	const query = `
		INSERT INTO leads (title, description, owner_id, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`

	var id int64
	err := r.db.QueryRow(
		query,
		lead.Title,
		lead.Description,
		lead.OwnerID,
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
		    status = $4
		WHERE id = $5
	`
	_, err := r.db.Exec(
		query,
		lead.Title,
		lead.Description,
		lead.OwnerID,
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
	const query = `
		SELECT id, title, description, created_at, owner_id, status
		FROM leads
		WHERE id = $1
	`
	row := r.db.QueryRow(query, id)
	lead := &models.Leads{}
	if err := row.Scan(
		&lead.ID,
		&lead.Title,
		&lead.Description,
		&lead.CreatedAt,
		&lead.OwnerID,
		&lead.Status,
	); err != nil {
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

func (r *LeadRepository) CountLeads() (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM leads`).Scan(&count)
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

	query := "SELECT id, title, description, created_at, owner_id, status FROM leads WHERE 1=1"
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
		var l models.Leads
		if err := rows.Scan(&l.ID, &l.Title, &l.Description, &l.CreatedAt, &l.OwnerID, &l.Status); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

func (r *LeadRepository) ListAll(limit, offset int) ([]*models.Leads, error) {
	const query = `
		SELECT id, title, description, created_at, owner_id, status
		FROM leads
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := r.db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.Leads
	for rows.Next() {
		var l models.Leads
		if err := rows.Scan(&l.ID, &l.Title, &l.Description, &l.CreatedAt, &l.OwnerID, &l.Status); err != nil {
			return nil, err
		}
		out = append(out, &l)
	}
	return out, nil
}

func (r *LeadRepository) ListPaginated(limit, offset int) ([]*models.Leads, error) {
	return r.ListAll(limit, offset)
}

// «Только мои» лиды
func (r *LeadRepository) ListByOwner(ownerID, limit, offset int) ([]*models.Leads, error) {
	const query = `
		SELECT id, title, description, created_at, owner_id, status
		FROM leads
		WHERE owner_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.Query(query, ownerID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.Leads
	for rows.Next() {
		var l models.Leads
		if err := rows.Scan(&l.ID, &l.Title, &l.Description, &l.CreatedAt, &l.OwnerID, &l.Status); err != nil {
			return nil, err
		}
		out = append(out, &l)
	}
	return out, nil
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
func (r *LeadRepository) GetLeadsSummaryStats(ctx context.Context, from, to time.Time, ownerID *int) ([]models.LeadSummaryRow, error) {
	query := `SELECT status, '' AS source, COUNT(*) AS count FROM leads WHERE created_at BETWEEN $1 AND $2`
	args := []interface{}{from, to}

	if ownerID != nil {
		query += " AND owner_id = $3"
		args = append(args, *ownerID)
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
