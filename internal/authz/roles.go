package authz

const (
	RoleSales       = 10
	RoleControl     = 30 // quality_control
	RoleManagement  = 40
	RoleSystemAdmin = 50
	RoleVisa        = 60
	RolePartner     = 70
	RoleHR          = 80
	RoleLegal       = 90

	// Backward-compatible alias: historically id=50 was treated as admin-staff.
	RoleAdminStaff = RoleSystemAdmin
)

type RoleMeta struct {
	ID             int
	Code           string
	LegacyName     string
	DisplayName    string
	IsSystemRole   bool
	IsBusinessRole bool
	ReadOnly       bool
}

var Roles = map[int]RoleMeta{
	RoleSales: {
		ID:             RoleSales,
		Code:           "sales",
		LegacyName:     "sales",
		DisplayName:    "Менеджер по продажам (МОП)",
		IsBusinessRole: true,
	},
	RoleControl: {
		ID:             RoleControl,
		Code:           "quality_control",
		LegacyName:     "audit",
		DisplayName:    "Отдел контроля качества",
		IsBusinessRole: true,
		ReadOnly:       true,
	},
	RoleManagement: {
		ID:             RoleManagement,
		Code:           "management",
		LegacyName:     "management",
		DisplayName:    "Руководство",
		IsBusinessRole: true,
	},
	RoleSystemAdmin: {
		ID:          RoleSystemAdmin,
		Code:        "admin",
		LegacyName:  "admin",
		DisplayName: "Администратор",
		IsSystemRole: true,
	},
	RoleVisa: {
		ID:             RoleVisa,
		Code:           "visa",
		LegacyName:     "visa",
		DisplayName:    "Визовый отдел",
		IsBusinessRole: true,
	},
	RolePartner: {
		ID:             RolePartner,
		Code:           "partner",
		LegacyName:     "partner",
		DisplayName:    "Менеджер по партнёрам",
		IsBusinessRole: true,
	},
	RoleHR: {
		ID:             RoleHR,
		Code:           "hr",
		LegacyName:     "hr",
		DisplayName:    "Отдел кадров",
		IsBusinessRole: true,
	},
	RoleLegal: {
		ID:             RoleLegal,
		Code:           "legal",
		LegacyName:     "legal",
		DisplayName:    "Юрист",
		IsBusinessRole: true,
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
	return roleID == RoleManagement || roleID == RoleSystemAdmin ||
		roleID == RoleVisa || roleID == RolePartner ||
		roleID == RoleHR || roleID == RoleLegal ||
		roleID == RoleControl // quality_control: their primary job is reviewing/approving documents
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

// CanManageFunnels controls who may create/edit/delete funnels, stages, and
// funnel transition rules. Only system admins and management can do this.
func CanManageFunnels(roleID int) bool {
	return roleID == RoleSystemAdmin || roleID == RoleManagement
}
