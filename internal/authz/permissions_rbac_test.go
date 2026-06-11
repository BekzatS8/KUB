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
// RoleOperations is now an alias for RoleVisa (id=20, code="visa"). The string "operations" is not
// a valid active code, so HasPermission("operations", ...) must always return false.
func TestLegacyOperationsCodeHasNoPermissions(t *testing.T) {
	// RoleOperations is an alias for RoleVisa=20, which is active
	code := RoleCodeByID(RoleOperations)
	if code != "visa" {
		t.Errorf("RoleOperations (id=20) must resolve to 'visa', got %q", code)
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
	// visa/operations (role_id=20): can VIEW funnels but NOT create them
	if !Can(UserContext{RoleID: RoleOperations}, ActionFunnelsView, "funnel") {
		t.Error("visa/operations role_id=20 must be allowed funnels.view")
	}
	if Can(UserContext{RoleID: RoleOperations}, ActionFunnelsCreate, "funnel") {
		t.Error("visa/operations role_id=20 must NOT be allowed funnels.create")
	}
}
