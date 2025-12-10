package authz

const (
	RoleSales      = 10
	RoleOperations = 20
	RoleControl    = 30
	RoleManagement = 40
	RoleAdminStaff = 50
)

func IsElevated(roleID int) bool {
	return roleID == RoleOperations || roleID == RoleManagement || roleID == RoleControl
}

func IsReadOnly(roleID int) bool {
	return roleID == RoleControl
}

func IsFullAccess(roleID int) bool {
	return roleID == RoleManagement
}

func CanAccessTasks(roleID int) bool {
	switch roleID {
	case RoleManagement, RoleOperations, RoleControl, RoleSales, RoleAdminStaff:
		return true
	default:
		return false
	}
}
