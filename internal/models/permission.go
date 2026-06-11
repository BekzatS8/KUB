package models

type PermissionAssignment struct {
	Action string `json:"action"`
	Scope  string `json:"scope"`
}

type PermissionPrincipal struct {
	UserID         int         `json:"user_id"`
	RoleID         int         `json:"role_id"`
	RoleName       string      `json:"role_name"`
	RoleCode       string      `json:"role_code"`
	DepartmentID   *int        `json:"department_id,omitempty"`
	DepartmentCode string      `json:"department_code,omitempty"`
	Department     *Department `json:"department,omitempty"`
	BranchID       *int        `json:"branch_id,omitempty"`
	Branch         *Branch     `json:"branch,omitempty"`
}

type PermissionsMeResponse struct {
	Role         map[string]any         `json:"role"`
	Department   *Department            `json:"department,omitempty"`
	Branch       *Branch                `json:"branch,omitempty"`
	Permissions  []PermissionAssignment `json:"permissions"`
	Scopes       map[string]string      `json:"scopes"`
	MenuSections []string               `json:"menu_sections"`
}
