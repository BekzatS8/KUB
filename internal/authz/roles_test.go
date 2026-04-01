package authz

import "testing"

func TestLeadershipHasFullBusinessAccess(t *testing.T) {
	if !CanViewAllBusinessData(RoleManagement) {
		t.Fatalf("leadership must have full business access")
	}
	if !CanProcessDocuments(RoleManagement) {
		t.Fatalf("leadership must process documents")
	}
}

func TestSystemAdminCanManageSystem(t *testing.T) {
	if !CanManageSystem(RoleSystemAdmin) || !CanAssignRoles(RoleSystemAdmin) || !CanAccessLogs(RoleSystemAdmin) || !CanManageIntegrations(RoleSystemAdmin) {
		t.Fatalf("system admin must manage system")
	}
	if CanViewAllBusinessData(RoleSystemAdmin) {
		t.Fatalf("system admin must not automatically have full business data access")
	}
}

func TestControlRestrictions(t *testing.T) {
	if !CanViewAllBusinessData(RoleControl) {
		t.Fatalf("control must view broad business data")
	}
	if CanViewLeadershipData(RoleControl) {
		t.Fatalf("control must not view leadership data")
	}
	if CanAssignRoles(RoleControl) {
		t.Fatalf("control must not assign roles")
	}
}

func TestOperationsAndSalesScopes(t *testing.T) {
	if !CanProcessDocuments(RoleOperations) {
		t.Fatalf("operations must process documents")
	}
	if CanManageSystem(RoleOperations) {
		t.Fatalf("operations must not manage system")
	}
	if !CanWorkWithLeads(RoleSales) {
		t.Fatalf("sales must work with leads")
	}
	if CanManageSystem(RoleSales) {
		t.Fatalf("sales must not manage system")
	}
}

func TestBackwardCompatibilityForRole50(t *testing.T) {
	if RoleAdminStaff != RoleSystemAdmin {
		t.Fatalf("legacy role alias must point to system admin")
	}
	if !CanManageSystem(RoleAdminStaff) {
		t.Fatalf("legacy id=50 should keep system admin semantics")
	}
}

func TestNegativeUnknownRole(t *testing.T) {
	unknown := 999
	if IsKnownRole(unknown) {
		t.Fatalf("unknown role should not be known")
	}
	if CanManageSystem(unknown) || CanViewAllBusinessData(unknown) || CanProcessDocuments(unknown) || CanWorkWithLeads(unknown) {
		t.Fatalf("unknown role must have no permissions")
	}
}
