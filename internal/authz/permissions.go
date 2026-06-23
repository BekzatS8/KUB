package authz

import (
	"strings"

	"turcompany/internal/models"
)

const (
	ActionFeedView                = "feed.view"
	ActionFunnelsView             = "funnels.view"
	ActionFunnelsCreate           = "funnels.create"
	ActionFunnelsUpdate           = "funnels.update"
	ActionFunnelsDelete           = "funnels.delete"
	ActionFunnelsReorder          = "funnels.reorder"
	ActionLeadsMoveBetweenFunnels = "leads.move_between_funnels"
)

type UserContext struct {
	UserID         int
	RoleID         int
	RoleCode       string
	DepartmentID   *int
	DepartmentCode string
	BranchID       *int
}

type Permission struct {
	Action string
	Scope  string
}

var allActions = []string{
	"feed.view",
	"leads.view", "leads.create", "leads.update", "leads.delete", "leads.transfer_manager", "leads.move_between_funnels",
	"deals.view", "deals.create", "deals.update", "deals.delete",
	"clients.view", "clients.create", "clients.update", "clients.delete", "clients.export",
	"documents.view", "documents.create", "documents.update", "documents.delete", "documents.send", "documents.download",
	"tasks.view", "tasks.create", "tasks.update", "tasks.delete",
	"users.view", "users.create", "users.update", "users.delete", "users.block", "users.move_department", "users.move_branch",
	"branches.view", "branches.create", "branches.update", "branches.delete",
	"funnels.view", "funnels.create", "funnels.update", "funnels.delete", "funnels.reorder",
	"reports.view",
	"chat.view", "chat.delete",
	"messenger.view",
	"telephony.view",
	"approvals.view", "approvals.create", "approvals.approve", "approvals.reject",
}

var baseRolePermissions = map[string][]Permission{
	"admin": permissionsForScope(ScopeAll, allActions...),
	"management": append(
		permissionsForScope(ScopeRelatedDepartments,
			"feed.view", "leads.view", "leads.create", "leads.update", "leads.transfer_manager", "leads.move_between_funnels",
			"deals.view", "deals.create", "deals.update",
			"clients.view", "clients.create", "clients.update",
			"documents.view", "documents.create", "documents.update", "documents.send",
			"tasks.view", "tasks.create", "tasks.update", "reports.view", "chat.view", "messenger.view", "telephony.view", "funnels.view",
			"users.view", "users.create", "users.update", "users.delete", "users.block",
			"branches.view",
		),
		Permission{Action: "approvals.create", Scope: ScopeOwn},
	),
	"quality_control": append(
		permissionsForScope(ScopeRelatedDepartments,
			"feed.view", "leads.view", "deals.view", "clients.view",
			"documents.view",
			"tasks.view", "reports.view", "chat.view", "messenger.view", "telephony.view", "funnels.view",
		),
		permissionsForScope(ScopeDepartment,
			"documents.create", "documents.update", "documents.send", "documents.download",
		)...,
	),
	"sales": permissionsForScope(ScopeDepartment,
		"feed.view", "leads.view", "leads.create", "leads.update", "deals.view", "deals.create", "deals.update",
		"clients.view", "clients.create", "clients.update", "documents.view", "documents.send",
		"tasks.view", "tasks.create", "tasks.update", "chat.view", "messenger.view", "telephony.view", "funnels.view", "approvals.create",
	),
	// visa: no deals.* (visa dept handles leads/documents/clients only, not sales deals)
	// visa: documents — view+send only (no create/update/download)
	"visa": permissionsForScope(ScopeDepartment,
		"feed.view", "leads.view", "leads.create", "leads.update", "clients.view", "clients.update",
		"documents.view", "documents.send",
		"tasks.view", "tasks.create", "tasks.update", "chat.view", "messenger.view", "telephony.view", "funnels.view", "approvals.create",
	),
	// partner: no deals.* (partner dept works with leads/clients only)
	// partner cannot add, send, or download documents — only view them (read-only document access)
	// partner sees all clients (общая база) and all dept leads (by funnel/department)
	"partner": permissionsForScope(ScopeDepartment,
		"feed.view", "leads.view", "leads.create", "leads.update", "clients.view", "clients.create", "clients.update",
		"documents.view",
		"tasks.view", "tasks.create", "tasks.update", "chat.view", "messenger.view", "telephony.view", "funnels.view", "approvals.create",
	),
	// hr: employee/document management; no leads/deals/messenger
	"hr": append(
		permissionsForScope(ScopeDepartment,
			"feed.view",
			"users.view", "users.create", "users.update", "users.delete", "users.block",
			"documents.view", "documents.create", "documents.update", "documents.send", "documents.download",
			"tasks.view", "tasks.create", "tasks.update", "chat.view", "telephony.view", "approvals.create",
		),
		// branches.view is needed for HR to list branches when creating/assigning users
		Permission{Action: "branches.view", Scope: ScopeAll},
	),
	// legal: clients+documents+users access; no leads/deals/messenger
	"legal": permissionsForScope(ScopeDepartment,
		"feed.view", "clients.view",
		"users.view", "users.create", "users.update", "users.delete", "users.block",
		"documents.view", "documents.create", "documents.update", "documents.send", "documents.download",
		"tasks.view", "tasks.create", "tasks.update", "chat.view", "telephony.view", "approvals.create",
	),
}

func permissionsForScope(scope string, actions ...string) []Permission {
	out := make([]Permission, 0, len(actions))
	for _, action := range actions {
		out = append(out, Permission{Action: action, Scope: scope})
	}
	return out
}

func PermissionsForRole(roleCode string) []Permission {
	roleCode = NormalizeRoleCode(strings.TrimSpace(strings.ToLower(roleCode)))
	perms := baseRolePermissions[roleCode]
	out := make([]Permission, len(perms))
	copy(out, perms)
	return out
}

func PermissionAssignmentsForRole(roleCode string) []models.PermissionAssignment {
	perms := PermissionsForRole(roleCode)
	out := make([]models.PermissionAssignment, 0, len(perms))
	for _, perm := range perms {
		out = append(out, models.PermissionAssignment{Action: perm.Action, Scope: perm.Scope})
	}
	return out
}

func PermissionScopesForRole(roleCode string) map[string]string {
	return PermissionScopes(PermissionsForRole(roleCode))
}

func PermissionScopes(perms []Permission) map[string]string {
	out := map[string]string{}
	for _, perm := range perms {
		if ScopeRank(perm.Scope) >= ScopeRank(out[perm.Action]) {
			out[perm.Action] = perm.Scope
		}
	}
	return out
}

func HasPermission(roleCode, action string) bool {
	action = strings.TrimSpace(action)
	if action == "" {
		return true
	}
	for _, perm := range PermissionsForRole(roleCode) {
		if perm.Action == action {
			return true
		}
	}
	return false
}

func Can(user UserContext, action, resource string) bool {
	roleCode := strings.TrimSpace(user.RoleCode)
	if roleCode == "" {
		roleCode = RoleCodeByID(user.RoleID)
	}
	return HasPermission(roleCode, action)
}

func MenuSectionsForRole(roleCode string) []string {
	return MenuSectionsForScopes(PermissionScopesForRole(roleCode))
}

func MenuSectionsForScopes(scopes map[string]string) []string {
	items := []struct {
		action string
		key    string
	}{
		{"feed.view", "feed"},
		{"reports.view", "reports"},
		{"documents.view", "documents"},
		{"tasks.view", "tasks"},
		{"clients.view", "clients"},
		{"chat.view", "chat"},
		{"messenger.view", "messenger"},
		{"telephony.view", "telephony"},
		{"leads.view", "sales.leads"},
		{"deals.view", "sales.deals"},
		{"users.view", "settings.users"},
		{"branches.view", "settings.branches"},
		{"funnels.view", "settings.funnels"},
		{"approvals.view", "approvals"},
	}
	out := []string{}
	for _, item := range items {
		if _, ok := scopes[item.action]; ok {
			out = append(out, item.key)
		}
	}
	return out
}
