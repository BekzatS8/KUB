package authz

import "testing"

// Передать лид (POST /leads/:id/assign) — админ и руководство; «Взять себе»
// (POST /leads/:id/take, leads.update) — все, кто работает с лидами.
func TestLeadOwnerRoutePermissions(t *testing.T) {
	cases := []struct {
		role             int
		transfer, update bool
	}{
		{RoleSystemAdmin, true, true},
		{RoleManagement, true, true},
		{RoleSales, false, true},
		{RoleVisa, false, true},
		{RolePartner, false, true},
	}
	for _, tc := range cases {
		u := UserContext{UserID: 1, RoleID: tc.role}
		if got := Can(u, "leads.transfer_manager", "lead"); got != tc.transfer {
			t.Errorf("role %d leads.transfer_manager = %v, want %v", tc.role, got, tc.transfer)
		}
		if got := Can(u, "leads.update", "lead"); got != tc.update {
			t.Errorf("role %d leads.update = %v, want %v", tc.role, got, tc.update)
		}
	}
}
