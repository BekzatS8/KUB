package repositories

import (
	"database/sql"
	"fmt"
	"log"

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

func (r *LeadRepository) Create(lead *models.Leads) error {
	const query = `
		INSERT INTO leads (title, description, created_at, owner_id, status)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.Exec(query, lead.Title, lead.Description, lead.CreatedAt, lead.OwnerID, lead.Status)
	return err
}

func (r *LeadRepository) Update(lead *models.Leads) error {
	const query = `
		UPDATE leads
		SET title=$1, description=$2, created_at=$3, owner_id=$4, status=$5
		WHERE id=$6
	`
	_, err := r.db.Exec(query, lead.Title, lead.Description, lead.CreatedAt, lead.OwnerID, lead.Status, lead.ID)
	return err
}

func (r *LeadRepository) GetByID(id int) (*models.Leads, error) {
	const query = `
		SELECT id, title, description, created_at, owner_id, status
		FROM leads
		WHERE id=$1
	`
	row := r.db.QueryRow(query, id)
	lead := &models.Leads{}
	if err := row.Scan(&lead.ID, &lead.Title, &lead.Description, &lead.CreatedAt, &lead.OwnerID, &lead.Status); err != nil {
		return nil, err
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

func (r *LeadRepository) ListPaginated(limit, offset int) ([]*models.Leads, error) {
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

// Новое: «только мои» лиды
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
