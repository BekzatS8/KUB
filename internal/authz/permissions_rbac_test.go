package authz

import "testing"

// TestFunnelCreatePermissions verifies that only admin can create funnels.
func TestFunnelCreatePermissions(t *testing.T) {
	allowed := []string{"admin"}
	denied := []string{"management", "quality_control", "sales", "visa", "partner", "hr", "legal", ""}

	for _, role := range allowed {
		if !HasPermission(role, ActionFunnelsCreate) {
			t.Errorf("role %q must be allowed to create funnels", role)
		}
	}
	for _, role := range denied {
		if HasPermission(role, ActionFunnelsCreate) {
			t.Errorf("role %q must NOT be allowed to create funnels", role)
		}
	}
}

// TestFunnelsReorderPermissions verifies that only admin can reorder funnels.
func TestFunnelsReorderPermissions(t *testing.T) {
	if !HasPermission("admin", ActionFunnelsReorder) {
		t.Error("admin must be allowed to reorder funnels")
	}
	for _, role := range []string{"management", "sales", "quality_control", "hr", "legal"} {
		if HasPermission(role, ActionFunnelsReorder) {
			t.Errorf("role %q must NOT reorder funnels", role)
		}
	}
}

// TestFunnelViewPermissions verifies which roles can view funnels.
func TestFunnelViewPermissions(t *testing.T) {
	canView := []string{"admin", "management", "quality_control", "sales", "visa", "partner"}
	cannotView := []string{"hr", "legal", ""}

	for _, role := range canView {
		if !HasPermission(role, ActionFunnelsView) {
			t.Errorf("role %q must be able to view funnels", role)
		}
	}
	for _, role := range cannotView {
		if HasPermission(role, ActionFunnelsView) {
			t.Errorf("role %q must NOT be able to view funnels", role)
		}
	}
}

// TestLeadsMoveBetweenFunnelsPermissions verifies the PATCH /leads/:id/funnel middleware gate.
func TestLeadsMoveBetweenFunnelsPermissions(t *testing.T) {
	canMove := []string{"admin", "management"}
	cannotMove := []string{"sales", "visa", "partner", "quality_control", "hr", "legal", ""}

	for _, role := range canMove {
		if !HasPermission(role, ActionLeadsMoveBetweenFunnels) {
			t.Errorf("role %q must be allowed to move leads between funnels", role)
		}
	}
	for _, role := range cannotMove {
		if HasPermission(role, ActionLeadsMoveBetweenFunnels) {
			t.Errorf("role %q must NOT be allowed to move leads between funnels", role)
		}
	}
}

// TestLegacyOperationsCodeHasNoPermissions ensures the string code "operations" gives no permissions.
// role_id=20 / code "operations" is a deleted legacy role. HasPermission("operations", ...) must always return false.
func TestLegacyOperationsCodeHasNoPermissions(t *testing.T) {
	// role_id=20 must not be a known role
	if IsKnownRole(20) {
		t.Errorf("role_id=20 (legacy operations) must NOT be a known role")
	}

	// The string code "operations" is not in baseRolePermissions — must return false for all actions
	for _, action := range allActions {
		if HasPermission("", action) {
			t.Errorf("empty role must NOT have permission for action %q", action)
		}
		if HasPermission("operations", action) {
			t.Errorf("legacy 'operations' string code must NOT have permission for action %q", action)
		}
	}
}

// TestAllActiveRolesHaveAtLeastOnePermission ensures every active role gets at least one permission.
func TestAllActiveRolesHaveAtLeastOnePermission(t *testing.T) {
	for id, meta := range Roles {
		perms := PermissionsForRole(meta.Code)
		if len(perms) == 0 {
			t.Errorf("active role id=%d code=%q has no permissions in baseRolePermissions", id, meta.Code)
		}
	}
}

// TestVisaHasNoDealsPermissions ensures visa dept cannot access sales deals.
func TestVisaHasNoDealsPermissions(t *testing.T) {
	for _, action := range []string{"deals.view", "deals.create", "deals.update", "deals.delete"} {
		if HasPermission("visa", action) {
			t.Errorf("visa must NOT have permission %q", action)
		}
	}
}

// TestPartnerHasNoDealsPermissions ensures partner dept cannot access sales deals.
func TestPartnerHasNoDealsPermissions(t *testing.T) {
	for _, action := range []string{"deals.view", "deals.create", "deals.update", "deals.delete"} {
		if HasPermission("partner", action) {
			t.Errorf("partner must NOT have permission %q", action)
		}
	}
}

// TestHRHasNoMessengerView ensures HR cannot access Wazzup/messenger.
func TestHRHasNoMessengerView(t *testing.T) {
	if HasPermission("hr", "messenger.view") {
		t.Error("hr must NOT have messenger.view")
	}
}

// TestLegalHasNoMessengerView ensures Legal cannot access Wazzup/messenger.
func TestLegalHasNoMessengerView(t *testing.T) {
	if HasPermission("legal", "messenger.view") {
		t.Error("legal must NOT have messenger.view")
	}
}

// TestQualityControlDocumentPermissions ensures QC can create/update/send docs (own dept) but cannot delete.
func TestQualityControlDocumentPermissions(t *testing.T) {
	allowed := []string{"documents.create", "documents.update", "documents.send", "documents.download", "documents.view"}
	for _, action := range allowed {
		if !HasPermission("quality_control", action) {
			t.Errorf("quality_control must have permission %q", action)
		}
	}
	if HasPermission("quality_control", "documents.delete") {
		t.Error("quality_control must NOT have documents.delete")
	}
}

// TestSalesHasNoDeleteOrExportPermissions ensures sales cannot delete entities or export clients.
func TestSalesHasNoDeleteOrExportPermissions(t *testing.T) {
	denied := []string{
		"leads.delete", "deals.delete", "clients.delete", "documents.delete",
		"tasks.delete", "chat.delete", "clients.export",
		"funnels.create", "funnels.update", "funnels.delete", "funnels.reorder",
	}
	for _, action := range denied {
		if HasPermission("sales", action) {
			t.Errorf("sales must NOT have permission %q", action)
		}
	}
}

// TestManagementHasLeadsMoveButNoFunnelsManagement ensures management can move leads but not manage funnel structure.
func TestManagementHasLeadsMoveButNoFunnelsManagement(t *testing.T) {
	if !HasPermission("management", "leads.move_between_funnels") {
		t.Error("management must have leads.move_between_funnels")
	}
	if !HasPermission("management", "leads.transfer_manager") {
		t.Error("management must have leads.transfer_manager")
	}
	for _, action := range []string{"funnels.create", "funnels.update", "funnels.delete", "funnels.reorder"} {
		if HasPermission("management", action) {
			t.Errorf("management must NOT have %q", action)
		}
	}
	if HasPermission("management", "clients.export") {
		t.Error("management must NOT have clients.export")
	}
}

// TestClientsRecordPermissions pins the target access model for the client record
// (the actions enforced by RequirePermission on the /clients routes):
//   - hr            : NO clients.view / create / update (no client access at all)
//   - legal         : clients.view only (sees all; cannot create/update the record)
//   - quality_control: clients.view only (read-only observer)
//   - visa          : clients.view + update (edits in scope; no create)
//   - partner       : clients.view + update (edits in scope; no create)
//   - sales         : clients.view + create + update (own/scope create+edit)
//   - management/admin: clients.view + create + update
func TestClientsRecordPermissions(t *testing.T) {
	type want struct{ view, create, update bool }
	cases := map[string]want{
		"hr":              {view: false, create: false, update: false},
		"legal":           {view: true, create: false, update: false},
		"quality_control": {view: true, create: false, update: false},
		"visa":            {view: true, create: false, update: true},
		"sales":           {view: true, create: true, update: true},
		"partner":         {view: true, create: false, update: true},
		"management":      {view: true, create: true, update: true},
		"admin":           {view: true, create: true, update: true},
	}
	for role, w := range cases {
		if got := HasPermission(role, "clients.view"); got != w.view {
			t.Errorf("role %q clients.view = %v, want %v", role, got, w.view)
		}
		if got := HasPermission(role, "clients.create"); got != w.create {
			t.Errorf("role %q clients.create = %v, want %v", role, got, w.create)
		}
		if got := HasPermission(role, "clients.update"); got != w.update {
			t.Errorf("role %q clients.update = %v, want %v", role, got, w.update)
		}
	}
}

// TestClientsDeleteIsAdminOnly ensures only admin holds clients.delete.
func TestClientsDeleteIsAdminOnly(t *testing.T) {
	if !HasPermission("admin", "clients.delete") {
		t.Error("admin must have clients.delete")
	}
	for _, role := range []string{"management", "quality_control", "sales", "visa", "partner", "hr", "legal", ""} {
		if HasPermission(role, "clients.delete") {
			t.Errorf("role %q must NOT have clients.delete", role)
		}
	}
}

// TestDealsCreatePermissions pins the target model for "who can create a deal",
// which is also the gate for converting a lead into a deal (PUT /leads/:id/convert*):
//   - sales / management / admin : YES
//   - visa / partner / quality_control / hr / legal : NO
func TestDealsCreatePermissions(t *testing.T) {
	canCreate := []string{"sales", "management", "admin"}
	cannotCreate := []string{"visa", "partner", "quality_control", "hr", "legal", ""}

	for _, role := range canCreate {
		if !HasPermission(role, "deals.create") {
			t.Errorf("role %q must have deals.create (lead→deal conversion)", role)
		}
	}
	for _, role := range cannotCreate {
		if HasPermission(role, "deals.create") {
			t.Errorf("role %q must NOT have deals.create (must get 403 on convert)", role)
		}
	}
}

// TestBranchesPermissions pins the target model for branches (Block B):
//   - branches.view   : admin + management (leadership oversight); also hr + legal,
//     who need a read-only branch list to assign a branch when creating user profiles
//     (they still cannot move users between branches — no users.move_branch)
//   - branches.create/update/delete : admin only
func TestBranchesPermissions(t *testing.T) {
	viewAllowed := []string{"admin", "management", "hr", "legal"}
	viewDenied := []string{"sales", "visa", "partner", "quality_control", ""}
	for _, role := range viewAllowed {
		if !HasPermission(role, "branches.view") {
			t.Errorf("role %q must have branches.view", role)
		}
	}
	for _, role := range viewDenied {
		if HasPermission(role, "branches.view") {
			t.Errorf("role %q must NOT have branches.view", role)
		}
	}

	for _, action := range []string{"branches.create", "branches.update", "branches.delete"} {
		if !HasPermission("admin", action) {
			t.Errorf("admin must have %q", action)
		}
		for _, role := range []string{"management", "sales", "visa", "partner", "quality_control", "hr", "legal"} {
			if HasPermission(role, action) {
				t.Errorf("role %q must NOT have %q (admin-only)", role, action)
			}
		}
	}
}

// TestAdminHasAllPermissions ensures admin has every defined action.
func TestAdminHasAllPermissions(t *testing.T) {
	for _, action := range allActions {
		if !HasPermission("admin", action) {
			t.Errorf("admin must have permission %q", action)
		}
	}
}

// TestNormalizeRoleCodeCoversActiveRoles ensures all active role codes survive normalization unchanged.
func TestNormalizeRoleCodeCoversActiveRoles(t *testing.T) {
	activeCodes := []string{"admin", "management", "quality_control", "sales", "visa", "partner", "hr", "legal"}
	for _, code := range activeCodes {
		got := NormalizeRoleCode(code)
		if got == "" {
			t.Errorf("NormalizeRoleCode(%q) returned empty string", code)
		}
	}
}

// TestTelephonyViewPermissions verifies that all active business roles have telephony.view,
// and that unknown / legacy roles do not.
func TestTelephonyViewPermissions(t *testing.T) {
	hasView := []string{"admin", "management", "quality_control", "sales", "visa", "partner", "hr", "legal"}
	for _, role := range hasView {
		if !HasPermission(role, "telephony.view") {
			t.Errorf("role %q must have telephony.view", role)
		}
	}
	noView := []string{"", "operations", "unknown_role"}
	for _, role := range noView {
		if HasPermission(role, "telephony.view") {
			t.Errorf("role %q must NOT have telephony.view", role)
		}
	}
}

// TestCanUsesCombinedRoleIDAndCode verifies that Can() resolves by RoleCode first, then falls back to RoleID.
func TestCanUsesCombinedRoleIDAndCode(t *testing.T) {
	// By code
	if !Can(UserContext{RoleCode: "admin"}, ActionFunnelsCreate, "funnel") {
		t.Error("admin by code must be allowed funnels.create")
	}
	// By ID fallback
	if !Can(UserContext{RoleID: RoleSystemAdmin}, ActionFunnelsCreate, "funnel") {
		t.Error("admin by role_id must be allowed funnels.create via ID fallback")
	}
	// Deny by code
	if Can(UserContext{RoleCode: "sales"}, ActionFunnelsCreate, "funnel") {
		t.Error("sales by code must NOT be allowed funnels.create")
	}
	// Deny by ID fallback
	if Can(UserContext{RoleID: RoleSales}, ActionFunnelsCreate, "funnel") {
		t.Error("sales by role_id must NOT be allowed funnels.create via ID fallback")
	}
	// visa (role_id=60): can VIEW funnels but NOT create them
	if !Can(UserContext{RoleID: RoleVisa}, ActionFunnelsView, "funnel") {
		t.Error("visa role_id=60 must be allowed funnels.view")
	}
	if Can(UserContext{RoleID: RoleVisa}, ActionFunnelsCreate, "funnel") {
		t.Error("visa role_id=60 must NOT be allowed funnels.create")
	}
}
