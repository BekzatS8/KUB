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
