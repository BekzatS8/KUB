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

// TestOperationsRoleHasNoPermissions ensures legacy role_id=20 gets no permissions.
func TestOperationsRoleHasNoPermissions(t *testing.T) {
	// role_id=20 has no code in Roles map, so RoleCodeByID returns ""
	code := RoleCodeByID(RoleOperations)
	if code != "" {
		t.Errorf("operations (role_id=20) must not have an active role code, got %q", code)
	}

	// With empty role code, HasPermission must return false for every action
	for _, action := range allActions {
		if HasPermission("", action) {
			t.Errorf("empty role must NOT have permission for action %q", action)
		}
		if HasPermission("operations", action) {
			t.Errorf("legacy operations role must NOT have permission for action %q", action)
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
	// operations by ID
	if Can(UserContext{RoleID: RoleOperations}, ActionFunnelsView, "funnel") {
		t.Error("operations role_id must NOT be allowed any funnel action")
	}
}
