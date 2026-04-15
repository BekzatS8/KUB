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

func TestCanManageIntegrationsForAllKnownRoles(t *testing.T) {
	allowed := []int{
		RoleSales,
		RoleOperations,
		RoleControl,
		RoleManagement,
		RoleSystemAdmin,
	}

	for _, roleID := range allowed {
		if !CanManageIntegrations(roleID) {
			t.Fatalf("role %d must be allowed to manage integrations", roleID)
		}
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

func TestRole50StaysSystemRole(t *testing.T) {
	meta, ok := Roles[RoleSystemAdmin]
	if !ok {
		t.Fatalf("role_id=50 must exist in role metadata")
	}
	if !meta.IsSystemRole {
		t.Fatalf("role_id=50 must stay marked as system role")
	}
	if meta.IsBusinessRole {
		t.Fatalf("role_id=50 must not be reclassified as business role")
	}
}

func TestRole50CanAccessAllBusinessDataViaNewHelper(t *testing.T) {
	if !CanAccessAllBusinessDataIncludingAdmin(RoleSystemAdmin) {
		t.Fatalf("role_id=50 must have full business access via new helper")
	}
}

func TestHardDeleteBusinessEntitiesOnlyForRole50(t *testing.T) {
	allowed := []int{RoleSystemAdmin}
	for _, roleID := range allowed {
		if !CanHardDeleteBusinessEntity(roleID) {
			t.Fatalf("role %d must be allowed for hard delete", roleID)
		}
	}

	denied := []int{RoleSales, RoleOperations, RoleControl, RoleManagement}
	for _, roleID := range denied {
		if CanHardDeleteBusinessEntity(roleID) {
			t.Fatalf("role %d must not be allowed for hard delete", roleID)
		}
	}
}

func TestReadOnlyRoleCannotArchiveOrHardDelete(t *testing.T) {
	if CanArchiveBusinessEntity(RoleControl) {
		t.Fatalf("read-only role must not archive business entities")
	}
	if CanHardDeleteBusinessEntity(RoleControl) {
		t.Fatalf("read-only role must not hard delete business entities")
	}
}

func TestNegativeUnknownRole(t *testing.T) {
	unknown := 999
	if IsKnownRole(unknown) {
		t.Fatalf("unknown role should not be known")
	}
	if CanManageSystem(unknown) || CanManageIntegrations(unknown) || CanViewAllBusinessData(unknown) || CanProcessDocuments(unknown) || CanWorkWithLeads(unknown) || CanArchiveBusinessEntity(unknown) || CanHardDeleteBusinessEntity(unknown) {
		t.Fatalf("unknown role must have no permissions")
	}
}
