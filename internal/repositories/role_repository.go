package repositories

import (
	"database/sql"
	"turcompany/internal/models"
)

type RoleRepository interface {
	GetByID(id int) (*models.Role, error)
	GetByName(name string) (*models.Role, error)
	List(limit, offset int) ([]*models.Role, error)
	Create(role *models.Role) error
	Update(role *models.Role) error
	Delete(id int) error
	GetCount() (int, error)
	GetRolesWithUserCounts() ([]map[string]interface{}, error)
}

type roleRepository struct {
	DB *sql.DB
}

func NewRoleRepository(db *sql.DB) RoleRepository {
	return &roleRepository{DB: db}
}

func (r *roleRepository) GetByID(id int) (*models.Role, error) {
	query := `SELECT id, name, description FROM roles WHERE id = $1`
	role := &models.Role{}
	err := r.DB.QueryRow(query, id).Scan(&role.ID, &role.Name, &role.Description)
	if err != nil {
		return nil, err
	}
	return role, nil
}

func (r *roleRepository) GetByName(name string) (*models.Role, error) {
	query := `SELECT id, name, description FROM roles WHERE name = $1`
	role := &models.Role{}
	err := r.DB.QueryRow(query, name).Scan(&role.ID, &role.Name, &role.Description)
	if err != nil {
		return nil, err
	}
	return role, nil
}

func (r *roleRepository) List(limit, offset int) ([]*models.Role, error) {
	query := `SELECT id, name, description FROM roles ORDER BY id LIMIT $1 OFFSET $2`
	rows, err := r.DB.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []*models.Role
	for rows.Next() {
		r := &models.Role{}
		if err := rows.Scan(&r.ID, &r.Name, &r.Description); err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	return roles, nil
}

func (r *roleRepository) Create(role *models.Role) error {
	query := `
		INSERT INTO roles (name, description)
		VALUES ($1, $2)
		RETURNING id
	`
	return r.DB.QueryRow(query, role.Name, role.Description).Scan(&role.ID)
}

func (r *roleRepository) Update(role *models.Role) error {
	query := `UPDATE roles SET name = $1, description = $2 WHERE id = $3`
	_, err := r.DB.Exec(query, role.Name, role.Description, role.ID)
	return err
}

func (r *roleRepository) Delete(id int) error {
	query := `DELETE FROM roles WHERE id = $1`
	_, err := r.DB.Exec(query, id)
	return err
}

func (r *roleRepository) GetCount() (int, error) {
	query := `SELECT COUNT(*) FROM roles`
	var count int
	err := r.DB.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *roleRepository) GetRolesWithUserCounts() ([]map[string]interface{}, error) {
	query := `
		SELECT
			r.id,
			r.name,
			r.description,
			COUNT(u.id) AS user_count
		FROM
			roles r
		LEFT JOIN
			users u ON r.id = u.role_id
		GROUP BY
			r.id
	`
	rows, err := r.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rolesWithCounts []map[string]interface{}
	for rows.Next() {
		var roleID int
		var roleName string
		var roleDescription string
		var userCount int
		if err := rows.Scan(&roleID, &roleName, &roleDescription, &userCount); err != nil {
			return nil, err
		}
		rolesWithCounts = append(rolesWithCounts, map[string]interface{}{
			"id":          roleID,
			"name":        roleName,
			"description": roleDescription,
			"user_count":  userCount,
		})
	}
	return rolesWithCounts, nil
}
