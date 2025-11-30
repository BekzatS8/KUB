package repositories

import (
	"database/sql"
	"fmt"
	"strings"

	"turcompany/internal/models"
)

type ClientRepository struct {
	db *sql.DB
}

func NewClientRepository(db *sql.DB) *ClientRepository {
	return &ClientRepository{db: db}
}

func (r *ClientRepository) Create(client *models.Client) (int64, error) {
	const q = `
                INSERT INTO clients (name, bin_iin, address, contact_info, created_at)
                VALUES ($1, $2, $3, $4, $5)
                RETURNING id
        `
	var id int64
	if err := r.db.QueryRow(q, client.Name, client.BinIin, client.Address, client.ContactInfo, client.CreatedAt).Scan(&id); err != nil {
		return 0, fmt.Errorf("create client: %w", err)
	}
	return id, nil
}

func (r *ClientRepository) Update(client *models.Client) error {
	const q = `
                UPDATE clients
                SET name=$1, bin_iin=$2, address=$3, contact_info=$4
                WHERE id=$5
        `
	if _, err := r.db.Exec(q, client.Name, client.BinIin, client.Address, client.ContactInfo, client.ID); err != nil {
		return fmt.Errorf("update client: %w", err)
	}
	return nil
}

func (r *ClientRepository) GetByID(id int) (*models.Client, error) {
	const q = `
                SELECT id, name, bin_iin, address, contact_info, created_at
                FROM clients
                WHERE id=$1
        `
	var c models.Client
	if err := r.db.QueryRow(q, id).Scan(&c.ID, &c.Name, &c.BinIin, &c.Address, &c.ContactInfo, &c.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get client: %w", err)
	}
	return &c, nil
}

func (r *ClientRepository) GetByBIN(bin string) (*models.Client, error) {
	const q = `
                SELECT id, name, bin_iin, address, contact_info, created_at
                FROM clients
                WHERE bin_iin=$1
        `
	var c models.Client
	if err := r.db.QueryRow(q, bin).Scan(&c.ID, &c.Name, &c.BinIin, &c.Address, &c.ContactInfo, &c.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get client by bin: %w", err)
	}
	return &c, nil
}

func (r *ClientRepository) List(limit, offset int) ([]*models.Client, error) {
	const q = `
                SELECT id, name, bin_iin, address, contact_info, created_at
                FROM clients
                ORDER BY created_at DESC
                LIMIT $1 OFFSET $2
        `
	rows, err := r.db.Query(q, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list clients: %w", err)
	}
	defer rows.Close()

	var res []*models.Client
	for rows.Next() {
		var c models.Client
		if err := rows.Scan(&c.ID, &c.Name, &c.BinIin, &c.Address, &c.ContactInfo, &c.CreatedAt); err != nil {
			return nil, err
		}
		res = append(res, &c)
	}
	return res, nil
}

func (r *ClientRepository) FindByName(name string) ([]*models.Client, error) {
	const q = `
                SELECT id, name, bin_iin, address, contact_info, created_at
                FROM clients
                WHERE LOWER(name) LIKE $1
                ORDER BY created_at DESC
        `
	rows, err := r.db.Query(q, "%"+strings.ToLower(name)+"%")
	if err != nil {
		return nil, fmt.Errorf("find clients by name: %w", err)
	}
	defer rows.Close()

	var res []*models.Client
	for rows.Next() {
		var c models.Client
		if err := rows.Scan(&c.ID, &c.Name, &c.BinIin, &c.Address, &c.ContactInfo, &c.CreatedAt); err != nil {
			return nil, err
		}
		res = append(res, &c)
	}
	return res, nil
}
