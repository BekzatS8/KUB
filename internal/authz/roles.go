package authz

const (
	RoleSales      = 10
	RoleOperations = 20
	RoleAudit      = 30
	RoleManagement = 40
	RoleAdmin      = 50
)

func IsElevated(roleID int) bool {
	return roleID == RoleOperations || roleID == RoleManagement || roleID == RoleAdmin
}

func IsReadOnly(roleID int) bool {
	return roleID == RoleAudit
}
