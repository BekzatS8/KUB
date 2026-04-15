package authz

const (
	RoleSales           = 10
	RoleBackofficeStaff = 15
	RoleOperations      = 20
	RoleControl         = 30
	RoleManagement      = 40
	RoleSystemAdmin     = 50

	// Backward-compatible alias: historically id=50 was treated as admin-staff.
	RoleAdminStaff = RoleSystemAdmin
)

type RoleMeta struct {
	ID             int
	Code           string
	LegacyName     string
	IsSystemRole   bool
	IsBusinessRole bool
	ReadOnly       bool
}

var Roles = map[int]RoleMeta{
	RoleSales: {
		ID:             RoleSales,
		Code:           "sales",
		LegacyName:     "sales",
		IsBusinessRole: true,
	},
	RoleBackofficeStaff: {
		ID:             RoleBackofficeStaff,
		Code:           "backoffice_admin_staff",
		LegacyName:     "staff",
		IsBusinessRole: true,
	},
	RoleOperations: {
		ID:             RoleOperations,
		Code:           "operations",
		LegacyName:     "operations",
		IsBusinessRole: true,
	},
	RoleControl: {
		ID:             RoleControl,
		Code:           "control",
		LegacyName:     "audit",
		IsBusinessRole: true,
		ReadOnly:       true,
	},
	RoleManagement: {
		ID:             RoleManagement,
		Code:           "leadership",
		LegacyName:     "management",
		IsBusinessRole: true,
	},
	RoleSystemAdmin: {
		ID:           RoleSystemAdmin,
		Code:         "system_admin",
		LegacyName:   "admin",
		IsSystemRole: true,
	},
}

func IsKnownRole(roleID int) bool {
	_, ok := Roles[roleID]
	return ok
}

func IsElevated(roleID int) bool {
	return roleID == RoleOperations || roleID == RoleManagement || roleID == RoleControl || roleID == RoleSystemAdmin
}

func IsReadOnly(roleID int) bool {
	return roleID == RoleControl
}

func IsFullAccess(roleID int) bool {
	return roleID == RoleManagement
}

func CanManageSystem(roleID int) bool {
	return roleID == RoleSystemAdmin
}

func CanAssignRoles(roleID int) bool {
	return roleID == RoleSystemAdmin
}

func CanAccessLogs(roleID int) bool {
	return roleID == RoleSystemAdmin
}

func CanManageIntegrations(roleID int) bool {
	return IsKnownRole(roleID)
}

func CanViewLeadershipData(roleID int) bool {
	return roleID == RoleManagement || roleID == RoleSystemAdmin
}

func CanViewAllBusinessData(roleID int) bool {
	return roleID == RoleManagement || roleID == RoleControl || roleID == RoleOperations
}

func CanHardDeleteBusinessEntity(roleID int) bool {
	return roleID == RoleSystemAdmin
}

func CanArchiveBusinessEntity(roleID int) bool {
	if roleID == RoleSystemAdmin {
		return true
	}

	role, ok := Roles[roleID]
	if !ok {
		return false
	}

	return role.IsBusinessRole && !role.ReadOnly
}

func CanAccessAllBusinessDataIncludingAdmin(roleID int) bool {
	return CanViewAllBusinessData(roleID) || roleID == RoleSystemAdmin
}

func CanProcessDocuments(roleID int) bool {
	return roleID == RoleOperations || roleID == RoleManagement
}

func CanWorkWithLeads(roleID int) bool {
	switch roleID {
	case RoleSales, RoleOperations, RoleManagement:
		return true
	default:
		return false
	}
}

func CanAccessMessengerOnly(roleID int) bool {
	return roleID == RoleBackofficeStaff
}

func CanAccessTasks(roleID int) bool {
	switch roleID {
	case RoleManagement, RoleOperations, RoleControl, RoleSales, RoleBackofficeStaff, RoleSystemAdmin:
		return true
	default:
		return false
	}
}

func CanUseChat(roleID int) bool {
	switch roleID {
	case RoleManagement, RoleSystemAdmin:
		return true
	case RoleControl, RoleOperations, RoleSales, RoleBackofficeStaff:
		return true
	default:
		return false
	}
}

func CanSendChatMessage(roleID int) bool {
	return CanUseChat(roleID)
}

func CanWriteChat(roleID int) bool {
	return CanUseChat(roleID)
}

func CanStartPersonalChat(roleID int) bool {
	return CanUseChat(roleID)
}

func CanMarkReadChat(roleID int) bool {
	return CanUseChat(roleID)
}

func CanCreateChatGroup(roleID int) bool {
	return CanUseChat(roleID)
}

func CanViewChatParticipantProfile(roleID int) bool {
	return CanUseChat(roleID)
}
