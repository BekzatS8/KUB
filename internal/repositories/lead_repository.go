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
	var status sql.NullString

	if err := scanner.Scan(
		&lead.ID,
		&lead.Title,
		&description,
		&lead.CreatedAt,
		&lead.OwnerID,
		&status,
	); err != nil {
		return nil, err
	}

	lead.Description = stringFromNull(description)
	lead.Status = normalizeLeadStatus(status)
	return lead, nil
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
		l, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
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
		l, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
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
	query := `SELECT COALESCE(status, 'new') AS status, '' AS source, COUNT(*) AS count FROM leads WHERE created_at BETWEEN $1 AND $2`
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

	existing := &models.Deals{}
	var status sql.NullString
	row := tx.QueryRow(`
		SELECT id, lead_id, client_id, owner_id, amount, currency, status, created_at
		FROM deals
		WHERE lead_id = $1
		ORDER BY created_at DESC
		LIMIT 1
		FOR UPDATE
	`, leadID)
	if err = row.Scan(
		&existing.ID,
		&existing.LeadID,
		&existing.ClientID,
		&existing.OwnerID,
		&existing.Amount,
		&existing.Currency,
		&status,
		&existing.CreatedAt,
	); err == nil {
		existing.Status = normalizeDealStatus(status)
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
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check existing deal: %w", err)
	}

	leadStatusValue := normalizeLeadStatus(leadStatus)
	if leadStatusValue != "confirmed" {
		return nil, errors.New("lead is not in a convertible status")
	}

	if client == nil {
		return nil, errors.New("client data is required")
	}

	if client.OwnerID == 0 {
		client.OwnerID = deal.OwnerID
	}
	if client.CreatedAt.IsZero() {
		client.CreatedAt = time.Now()
	}

	var clientID int
	if err = tx.QueryRow(`
		SELECT id FROM clients
		WHERE ($1 <> '' AND bin_iin = $1)
		   OR ($2 <> '' AND iin = $2)
		   OR ($3 <> '' AND phone = $3)
		LIMIT 1
	`, client.BinIin, client.IIN, client.Phone).Scan(&clientID); err == nil {
		deal.ClientID = clientID
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("lookup client: %w", err)
	}

	if deal.ClientID == 0 {
		if client.BinIin != "" {
			insertClientQuery := `
				INSERT INTO clients (
					name, bin_iin, address, contact_info,
					last_name, first_name, middle_name,
					iin, id_number, passport_series, passport_number,
					phone, email, registration_address, actual_address,
					owner_id, created_at
				)
				VALUES (
					$1, $2, $3, $4,
					$5, $6, $7,
					$8, $9, $10, $11,
					$12, $13, $14, $15,
					$16, $17
				)
				ON CONFLICT (bin_iin) WHERE bin_iin IS NOT NULL AND bin_iin <> ''
				DO NOTHING
				RETURNING id
			`

			err = tx.QueryRow(
				insertClientQuery,
				client.Name,
				nullStringFromEmpty(client.BinIin),
				client.Address,
				client.ContactInfo,
				client.LastName,
				client.FirstName,
				client.MiddleName,
				nullStringFromEmpty(client.IIN),
				client.IDNumber,
				client.PassportSeries,
				client.PassportNumber,
				client.Phone,
				client.Email,
				client.RegistrationAddress,
				client.ActualAddress,
				client.OwnerID,
				client.CreatedAt,
			).Scan(&clientID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					if err = tx.QueryRow(`SELECT id FROM clients WHERE bin_iin = $1`, client.BinIin).Scan(&clientID); err != nil {
						return nil, fmt.Errorf("get client by bin/iin: %w", err)
					}
				} else {
					return nil, fmt.Errorf("insert client: %w", err)
				}
			}
		} else {
			insertClientQuery := `
				INSERT INTO clients (
					name, bin_iin, address, contact_info,
					last_name, first_name, middle_name,
					iin, id_number, passport_series, passport_number,
					phone, email, registration_address, actual_address,
					owner_id, created_at
				)
				VALUES (
					$1, $2, $3, $4,
					$5, $6, $7,
					$8, $9, $10, $11,
					$12, $13, $14, $15,
					$16, $17
				)
				RETURNING id
			`
			if err = tx.QueryRow(
				insertClientQuery,
				client.Name,
				nullStringFromEmpty(client.BinIin),
				client.Address,
				client.ContactInfo,
				client.LastName,
				client.FirstName,
				client.MiddleName,
				nullStringFromEmpty(client.IIN),
				client.IDNumber,
				client.PassportSeries,
				client.PassportNumber,
				client.Phone,
				client.Email,
				client.RegistrationAddress,
				client.ActualAddress,
				client.OwnerID,
				client.CreatedAt,
			).Scan(&clientID); err != nil {
				return nil, fmt.Errorf("insert client without bin: %w", err)
			}
		}
		deal.ClientID = clientID
	}

	err = tx.QueryRow(`
		INSERT INTO deals (lead_id, client_id, owner_id, amount, currency, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (lead_id) DO NOTHING
		RETURNING id
	`,
		deal.LeadID,
		deal.ClientID,
		deal.OwnerID,
		deal.Amount,
		deal.Currency,
		deal.Status,
		deal.CreatedAt,
	).Scan(&deal.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			row := tx.QueryRow(`
				SELECT id, lead_id, client_id, owner_id, amount, currency, status, created_at
				FROM deals
				WHERE lead_id = $1
				ORDER BY created_at DESC
				LIMIT 1
				FOR UPDATE
			`, leadID)
			if err = row.Scan(
				&existing.ID,
				&existing.LeadID,
				&existing.ClientID,
				&existing.OwnerID,
				&existing.Amount,
				&existing.Currency,
				&status,
				&existing.CreatedAt,
			); err != nil {
				return nil, fmt.Errorf("fetch existing deal after conflict: %w", err)
			}
			existing.Status = normalizeDealStatus(status)
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
