package repositories

import (
	"database/sql"
	"strings"

	"turcompany/internal/authz"
	"turcompany/internal/models"
)

type PermissionRepository struct {
	db *sql.DB
}

func NewPermissionRepository(db *sql.DB) *PermissionRepository {
	return &PermissionRepository{db: db}
}

func (r *PermissionRepository) GetPrincipal(userID int) (*models.PermissionPrincipal, error) {
	p := &models.PermissionPrincipal{}
	var roleName, roleCode sql.NullString
	var departmentID sql.NullInt64
	var departmentName, departmentCode sql.NullString
	var departmentActive sql.NullBool
	var branchID sql.NullInt64
	var branchName, branchCode sql.NullString
	var branchActive sql.NullBool

	err := r.db.QueryRow(`
		SELECT
			u.id,
			COALESCE(u.role_id, 0),
			r.name,
			r.code,
			u.department_id,
			d.name,
			d.code,
			d.is_active,
			u.branch_id,
			b.name,
			b.code,
			b.is_active
		FROM users u
		LEFT JOIN roles r ON r.id = u.role_id
		LEFT JOIN departments d ON d.id = u.department_id
		LEFT JOIN branches b ON b.id = u.branch_id
		WHERE u.id = $1
	`, userID).Scan(
		&p.UserID,
		&p.RoleID,
		&roleName,
		&roleCode,
		&departmentID,
		&departmentName,
		&departmentCode,
		&departmentActive,
		&branchID,
		&branchName,
		&branchCode,
		&branchActive,
	)
	if err != nil {
		return nil, err
	}

	p.RoleName = roleName.String
	p.RoleCode = authz.NormalizeRoleCode(strings.TrimSpace(roleCode.String))
	if p.RoleCode == "" {
		p.RoleCode = authz.RoleCodeByID(p.RoleID)
	}

	if departmentID.Valid {
		id := int(departmentID.Int64)
		p.DepartmentID = &id
		p.DepartmentCode = departmentCode.String
		p.Department = &models.Department{
			ID:       id,
			Name:     departmentName.String,
			Code:     departmentCode.String,
			IsActive: !departmentActive.Valid || departmentActive.Bool,
		}
	}
	if branchID.Valid {
		id := int(branchID.Int64)
		p.BranchID = &id
		p.Branch = &models.Branch{
			ID:       id,
			Name:     branchName.String,
			Code:     branchCode.String,
			IsActive: !branchActive.Valid || branchActive.Bool,
		}
	}

	return p, nil
}

func (r *PermissionRepository) ListRolePermissions(roleID int) ([]models.PermissionAssignment, error) {
	rows, err := r.db.Query(`
		SELECT p.code, rp.scope
		FROM role_permissions rp
		JOIN permissions p ON p.id = rp.permission_id
		WHERE rp.role_id = $1
		ORDER BY p.code
	`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.PermissionAssignment{}
	for rows.Next() {
		var perm models.PermissionAssignment
		if err := rows.Scan(&perm.Action, &perm.Scope); err != nil {
			return nil, err
		}
		out = append(out, perm)
	}
	return out, rows.Err()
}
