package authz

const (
	RoleLegal       = 1  // lawyer in production (id=1, name='lawyer')
	RoleHR          = 2  // hr in production (id=2, name='hr')
	RolePartner     = 3  // bdm in production (id=3, name='bdm')
	RoleSales       = 10 // sales, unchanged
	RoleVisa        = 20 // vds in production (id=20, name='vds')
	RoleControl     = 30 // audit in production (id=30, name='audit')
	RoleManagement  = 40 // unchanged
	RoleSystemAdmin = 50 // unchanged

	// Backward-compatible alias: historically id=50 was treated as admin-staff.
	RoleAdminStaff = RoleSystemAdmin

	// RoleOperations is a deprecated alias for RoleVisa.
	// In production, role_id=20 is the visa department (vds).
	// All existing service code using RoleOperations was correctly handling
	// visa users at id=20. No service files need to change.
	RoleOperations = RoleVisa
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
	RoleLegal: {
		ID:             RoleLegal,
		Code:           "legal",
		LegacyName:     "lawyer",
		IsBusinessRole: true,
	},
	RoleHR: {
		ID:             RoleHR,
		Code:           "hr",
		LegacyName:     "hr",
		IsBusinessRole: true,
	},
	RolePartner: {
		ID:             RolePartner,
		Code:           "partner",
		LegacyName:     "bdm",
		IsBusinessRole: true,
	},
	RoleSales: {
		ID:             RoleSales,
		Code:           "sales",
		LegacyName:     "sales",
		IsBusinessRole: true,
	},
	RoleVisa: {
		ID:             RoleVisa,
		Code:           "visa",
		LegacyName:     "vds",
		IsBusinessRole: true,
	},
	RoleControl: {
		ID:             RoleControl,
		Code:           "quality_control",
		LegacyName:     "audit",
		IsBusinessRole: true,
		ReadOnly:       true,
	},
	RoleManagement: {
		ID:             RoleManagement,
		Code:           "management",
		LegacyName:     "management",
		IsBusinessRole: true,
	},
	RoleSystemAdmin: {
		ID:           RoleSystemAdmin,
		Code:         "admin",
		LegacyName:   "admin",
		IsSystemRole: true,
	},
}

func NormalizeRoleCode(code string) string {
	switch code {
	case "system_admin", "admin_staff", "admin":
		return "admin"
	case "leadership", "manager", "management":
		return "management"
	case "control", "audit", "quality_control":
		return "quality_control"
	default:
		return code
	}
}

func RoleCodeByID(roleID int) string {
	if meta, ok := Roles[roleID]; ok {
		return meta.Code
	}
	return ""
}

func IsKnownRole(roleID int) bool {
	_, ok := Roles[roleID]
	return ok
}

func IsElevated(roleID int) bool {
	return roleID == RoleManagement || roleID == RoleControl || roleID == RoleSystemAdmin
}

func IsReadOnly(roleID int) bool {
	return roleID == RoleControl
}

func IsFullAccess(roleID int) bool {
	return roleID == RoleManagement || roleID == RoleSystemAdmin
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
	return roleID == RoleManagement || roleID == RoleControl || roleID == RoleSystemAdmin
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
	return roleID == RoleManagement || roleID == RoleSystemAdmin || roleID == RoleVisa || roleID == RolePartner || roleID == RoleHR || roleID == RoleLegal
}

func CanWorkWithLeads(roleID int) bool {
	switch roleID {
	case RoleSales, RoleManagement, RoleSystemAdmin, RoleVisa, RolePartner:
		return true
	default:
		return false
	}
}

func CanAccessTasks(roleID int) bool {
	switch roleID {
	case RoleManagement, RoleControl, RoleSales, RoleSystemAdmin, RoleVisa, RolePartner, RoleHR, RoleLegal:
		return true
	default:
		return false
	}
}

func CanUseChat(roleID int) bool {
	switch roleID {
	case RoleManagement, RoleSystemAdmin:
		return true
	case RoleControl, RoleSales, RoleVisa, RolePartner, RoleHR, RoleLegal:
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
