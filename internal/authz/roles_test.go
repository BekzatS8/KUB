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
	if !CanViewAllBusinessData(RoleSystemAdmin) {
		t.Fatalf("system admin must have full business data access")
	}
	if !CanProcessDocuments(RoleSystemAdmin) || !CanWorkWithLeads(RoleSystemAdmin) || !IsFullAccess(RoleSystemAdmin) {
		t.Fatalf("system admin must be allowed to use all business functions")
	}
}

func TestCanManageIntegrationsForAllKnownRoles(t *testing.T) {
	allowed := []int{
		RoleSales,
		RoleControl,
		RoleManagement,
		RoleSystemAdmin,
		RoleVisa,
		RolePartner,
		RoleHR,
		RoleLegal,
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

func TestVisaRolePermissions(t *testing.T) {
	if !IsKnownRole(RoleVisa) {
		t.Fatalf("visa role (id=%d) must be a known active role", RoleVisa)
	}
	if RoleCodeByID(RoleVisa) != "visa" {
		t.Fatalf("role_id=%d must resolve to code 'visa', got %q", RoleVisa, RoleCodeByID(RoleVisa))
	}
	if CanManageSystem(RoleVisa) {
		t.Fatalf("visa must not have system admin privileges")
	}
	if CanHardDeleteBusinessEntity(RoleVisa) {
		t.Fatalf("visa must not hard delete business entities")
	}
	if !CanWorkWithLeads(RoleVisa) {
		t.Fatalf("visa role must be able to work with leads")
	}
	if !CanUseChat(RoleVisa) {
		t.Fatalf("visa role must be able to use chat")
	}
}

func TestSalesScopes(t *testing.T) {
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

	denied := []int{RoleSales, RoleVisa, RoleControl, RoleManagement}
	for _, roleID := range denied {
		if CanHardDeleteBusinessEntity(roleID) {
			t.Fatalf("role %d must not be allowed for hard delete", roleID)
		}
	}
}

func TestRole20IsNotCanonicalAndDenied(t *testing.T) {
	if IsKnownRole(20) {
		t.Fatalf("role_id=20 (legacy operations) must not be part of canonical roles")
	}
	if CanAccessTasks(20) || CanUseChat(20) || CanManageIntegrations(20) {
		t.Fatalf("role_id=20 must be denied by policy helpers")
	}
}

func TestRole15IsNotCanonicalAndDenied(t *testing.T) {
	if IsKnownRole(15) {
		t.Fatalf("role_id=15 must not be part of canonical roles")
	}
	if CanAccessTasks(15) || CanUseChat(15) || CanManageIntegrations(15) {
		t.Fatalf("role_id=15 must be denied by policy helpers")
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

// TestQualityControlIsReadOnlyObserver verifies that quality_control (RoleControl),
// while it may manage its own department documents, still cannot REVIEW/approve
// documents (CanProcessDocuments) and cannot hard-delete business entities.
func TestQualityControlIsReadOnlyObserver(t *testing.T) {
	if CanProcessDocuments(RoleControl) {
		t.Fatal("quality_control must NOT review/approve documents")
	}
	if CanHardDeleteBusinessEntity(RoleControl) {
		t.Fatal("quality_control must NOT hard-delete business entities")
	}
}

func TestAllRolesHaveCanonicalDisplayNames(t *testing.T) {
	want := map[int]string{
		RoleSales:       "Менеджер по продажам (МОП)",
		RoleControl:     "Отдел контроля качества",
		RoleManagement:  "Руководство",
		RoleSystemAdmin: "Администратор",
		RoleVisa:        "Визовый отдел",
		RolePartner:     "Менеджер по партнёрам",
		RoleHR:          "Отдел кадров",
		RoleLegal:       "Юрист",
	}
	if len(want) != len(Roles) {
		t.Errorf("Roles map has %d entries, want %d", len(Roles), len(want))
	}
	for id, name := range want {
		meta, ok := Roles[id]
		if !ok {
			t.Errorf("role_id=%d not found in Roles map", id)
			continue
		}
		if meta.DisplayName == "" {
			t.Errorf("role_id=%d (%s) has empty DisplayName", id, meta.Code)
		}
		if meta.DisplayName != name {
			t.Errorf("role_id=%d: want display_name=%q, got %q", id, name, meta.DisplayName)
		}
	}
}

// CANON LOCK: эти значения зафиксированы намеренно. Если тест упал — НЕ правь тест под код.
// Сначала убедись, что изменение role IDs действительно санкционировано. Поправь КОД, не тест.
func TestCanonicalRoleIDsLocked(t *testing.T) {
	// --- Часть 1: литеральные значения констант ---
	// Сравнение с целыми литералами, не с другими константами — тавтология исключена.
	if RoleSales != 10 {
		t.Errorf("RoleSales: want 10, got %d", RoleSales)
	}
	if RoleControl != 30 {
		t.Errorf("RoleControl: want 30, got %d", RoleControl)
	}
	if RoleManagement != 40 {
		t.Errorf("RoleManagement: want 40, got %d", RoleManagement)
	}
	if RoleSystemAdmin != 50 {
		t.Errorf("RoleSystemAdmin: want 50, got %d", RoleSystemAdmin)
	}
	if RoleVisa != 60 {
		t.Errorf("RoleVisa: want 60, got %d", RoleVisa)
	}
	if RolePartner != 70 {
		t.Errorf("RolePartner: want 70, got %d", RolePartner)
	}
	if RoleHR != 80 {
		t.Errorf("RoleHR: want 80, got %d", RoleHR)
	}
	if RoleLegal != 90 {
		t.Errorf("RoleLegal: want 90, got %d", RoleLegal)
	}

	// --- Часть 2: маппинг id → code по литеральным id ---
	canonCodes := map[int]string{
		10: "sales",
		30: "quality_control",
		40: "management",
		50: "admin",
		60: "visa",
		70: "partner",
		80: "hr",
		90: "legal",
	}
	for id, wantCode := range canonCodes {
		got := RoleCodeByID(id)
		if got != wantCode {
			t.Errorf("RoleCodeByID(%d): want %q, got %q", id, wantCode, got)
		}
	}

	// --- Часть 3: role_id=20 не каноничен ---
	if IsKnownRole(20) {
		t.Errorf("role_id=20 (legacy operations) must NOT be a known role")
	}
	if code := RoleCodeByID(20); code != "" {
		t.Errorf("RoleCodeByID(20): want empty string, got %q", code)
	}

	// --- Часть 4: var Roles содержит ровно 8 активных ролей ---
	wantIDs := []int{10, 30, 40, 50, 60, 70, 80, 90}
	if len(Roles) != len(wantIDs) {
		t.Errorf("Roles map: want %d entries, got %d", len(wantIDs), len(Roles))
	}
	for _, id := range wantIDs {
		if _, ok := Roles[id]; !ok {
			t.Errorf("Roles map: missing canonical id=%d", id)
		}
	}
	if _, badEntry := Roles[20]; badEntry {
		t.Errorf("Roles map: must NOT contain id=20 (legacy operations)")
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
