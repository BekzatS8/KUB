package services

import (
	"testing"

	"turcompany/internal/authz"
	"turcompany/internal/models"
)

// ─── resolveLeadScope ────────────────────────────────────────────────────────

func TestResolveLeadScope_AdminAndManagementReturnAll(t *testing.T) {
	for _, roleID := range []int{authz.RoleSystemAdmin, authz.RoleManagement} {
		scope, err := resolveLeadScope(1, roleID, nil)
		if err != nil {
			t.Errorf("role %d: unexpected error: %v", roleID, err)
		}
		if scope.Kind != ScopeKindAll {
			t.Errorf("role %d: expected ScopeKindAll, got %v", roleID, scope.Kind)
		}
	}
}

func TestResolveLeadScope_SalesVisaPartnerReturnBranch(t *testing.T) {
	branchID := 3
	userRepo := &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}

	// quality_control is no longer branch-scoped — see TestQualityControlScopeIsAll.
	// partner is now department-scoped (same as sales/visa), not own-scoped.
	for _, roleID := range []int{authz.RoleSales, authz.RoleVisa, authz.RolePartner} {
		scope, err := resolveLeadScope(100, roleID, userRepo)
		if err != nil {
			t.Errorf("role %d: unexpected error: %v", roleID, err)
		}
		if scope.Kind != ScopeKindBranch {
			t.Errorf("role %d: expected ScopeKindBranch, got %v", roleID, scope.Kind)
		}
		if scope.BranchID == nil || *scope.BranchID != branchID {
			t.Errorf("role %d: expected branchID=%d, got %+v", roleID, branchID, scope.BranchID)
		}
	}
}

func TestResolveLeadScope_HRAndLegalAndUnknownReturnForbidden(t *testing.T) {
	for _, roleID := range []int{authz.RoleHR, authz.RoleLegal, 999} {
		scope, err := resolveLeadScope(1, roleID, nil)
		if err == nil {
			t.Errorf("role %d: expected ErrForbidden, got nil", roleID)
		}
		if scope.Kind != ScopeKindForbidden {
			t.Errorf("role %d: expected ScopeKindForbidden, got %v", roleID, scope.Kind)
		}
	}
}

// ─── resolveClientScope ──────────────────────────────────────────────────────

func TestResolveClientScope_AdminManagementLegalPartnerReturnAll(t *testing.T) {
	// partner now uses общая база (ScopeKindAll) for clients.
	for _, roleID := range []int{authz.RoleSystemAdmin, authz.RoleManagement, authz.RoleLegal, authz.RolePartner} {
		scope, err := resolveClientScope(1, roleID, nil)
		if err != nil {
			t.Errorf("role %d: unexpected error: %v", roleID, err)
		}
		if scope.Kind != ScopeKindAll {
			t.Errorf("role %d: expected ScopeKindAll, got %v", roleID, scope.Kind)
		}
	}
}

func TestResolveClientScope_SalesVisaReturnAll(t *testing.T) {
	// sales/visa have общая база (ScopeKindAll) for clients — no branch restriction.
	for _, roleID := range []int{authz.RoleSales, authz.RoleVisa} {
		scope, err := resolveClientScope(100, roleID, nil)
		if err != nil {
			t.Errorf("role %d: unexpected error: %v", roleID, err)
		}
		if scope.Kind != ScopeKindAll {
			t.Errorf("role %d: expected ScopeKindAll for clients, got %v", roleID, scope.Kind)
		}
	}
}

func TestResolveClientScope_HRAndUnknownReturnForbidden(t *testing.T) {
	for _, roleID := range []int{authz.RoleHR, 999} {
		scope, err := resolveClientScope(1, roleID, nil)
		if err == nil {
			t.Errorf("role %d: expected ErrForbidden, got nil", roleID)
		}
		if scope.Kind != ScopeKindForbidden {
			t.Errorf("role %d: expected ScopeKindForbidden, got %v", roleID, scope.Kind)
		}
	}
}

// ─── leadMatchesScope ────────────────────────────────────────────────────────

func TestLeadMatchesScope_AllScopeMatchesAnyLead(t *testing.T) {
	b := 99
	lead := &models.Leads{ID: 1, OwnerID: 5, BranchID: &b}
	scope := DataScope{Kind: ScopeKindAll}
	if !leadMatchesScope(scope, lead) {
		t.Error("ScopeKindAll must match any lead")
	}
}

func TestLeadMatchesScope_OwnScopeMatchesOwnerOnly(t *testing.T) {
	const partnerID = 42
	ownLead := &models.Leads{ID: 1, OwnerID: partnerID}
	foreignLead := &models.Leads{ID: 2, OwnerID: 99}
	scope := DataScope{Kind: ScopeKindOwn, UserID: partnerID}

	if !leadMatchesScope(scope, ownLead) {
		t.Error("partner must match own lead")
	}
	if leadMatchesScope(scope, foreignLead) {
		t.Error("partner must NOT match foreign lead")
	}
}

func TestLeadMatchesScope_BranchScopeMatchesSameBranchOnly(t *testing.T) {
	branchA, branchB := 3, 7
	scope := DataScope{Kind: ScopeKindBranch, BranchID: &branchA}

	sameBranchLead := &models.Leads{ID: 1, BranchID: &branchA}
	otherBranchLead := &models.Leads{ID: 2, BranchID: &branchB}
	noBranchLead := &models.Leads{ID: 3, BranchID: nil}

	if !leadMatchesScope(scope, sameBranchLead) {
		t.Error("branch scope must match same-branch lead")
	}
	if leadMatchesScope(scope, otherBranchLead) {
		t.Error("branch scope must NOT match different-branch lead")
	}
	if leadMatchesScope(scope, noBranchLead) {
		t.Error("branch scope must NOT match lead with no branch")
	}
}

func TestLeadMatchesScope_ForbiddenScopeMatchesNothing(t *testing.T) {
	b := 1
	lead := &models.Leads{ID: 1, OwnerID: 1, BranchID: &b}
	scope := DataScope{Kind: ScopeKindForbidden}
	if leadMatchesScope(scope, lead) {
		t.Error("ScopeKindForbidden must never match")
	}
}

func TestLeadMatchesScope_NilLeadReturnsFalse(t *testing.T) {
	scope := DataScope{Kind: ScopeKindAll}
	if leadMatchesScope(scope, nil) {
		t.Error("nil lead must return false for any scope")
	}
}

// ─── clientMatchesScope ──────────────────────────────────────────────────────

func TestClientMatchesScope_AllScopeMatchesAnyClient(t *testing.T) {
	b := 99
	client := &models.Client{ID: 1, OwnerID: 5, BranchID: &b}
	scope := DataScope{Kind: ScopeKindAll}
	if !clientMatchesScope(scope, client) {
		t.Error("ScopeKindAll must match any client")
	}
}

func TestClientMatchesScope_OwnScopeMatchesOwnerOnly(t *testing.T) {
	const partnerID = 42
	ownClient := &models.Client{ID: 1, OwnerID: partnerID}
	foreignClient := &models.Client{ID: 2, OwnerID: 99}
	scope := DataScope{Kind: ScopeKindOwn, UserID: partnerID}

	if !clientMatchesScope(scope, ownClient) {
		t.Error("partner must match own client")
	}
	if clientMatchesScope(scope, foreignClient) {
		t.Error("partner must NOT match foreign client")
	}
}

func TestClientMatchesScope_BranchScopeMatchesSameBranchOnly(t *testing.T) {
	branchA, branchB := 3, 7
	scope := DataScope{Kind: ScopeKindBranch, BranchID: &branchA}

	sameBranchClient := &models.Client{ID: 1, BranchID: &branchA}
	otherBranchClient := &models.Client{ID: 2, BranchID: &branchB}
	noBranchClient := &models.Client{ID: 3, BranchID: nil}

	if !clientMatchesScope(scope, sameBranchClient) {
		t.Error("branch scope must match same-branch client")
	}
	if clientMatchesScope(scope, otherBranchClient) {
		t.Error("branch scope must NOT match different-branch client")
	}
	if clientMatchesScope(scope, noBranchClient) {
		t.Error("branch scope must NOT match client with no branch")
	}
}

func TestClientMatchesScope_ForbiddenScopeMatchesNothing(t *testing.T) {
	b := 1
	client := &models.Client{ID: 1, OwnerID: 1, BranchID: &b}
	scope := DataScope{Kind: ScopeKindForbidden}
	if clientMatchesScope(scope, client) {
		t.Error("ScopeKindForbidden must never match")
	}
}

func TestClientMatchesScope_NilClientReturnsFalse(t *testing.T) {
	scope := DataScope{Kind: ScopeKindAll}
	if clientMatchesScope(scope, nil) {
		t.Error("nil client must return false for any scope")
	}
}

// Block D: a branch scope with no resolved branch must fail closed (deny),
// never allow-all. Hardening against a half-built scope.
func TestClientMatchesScope_BranchScopeNilBranchFailsClosed(t *testing.T) {
	scope := DataScope{Kind: ScopeKindBranch, BranchID: nil}
	b := 7
	client := &models.Client{ID: 1, BranchID: &b}
	if clientMatchesScope(scope, client) {
		t.Error("branch scope with nil BranchID must DENY (fail-closed), not allow-all")
	}
	clientNoBranch := &models.Client{ID: 2, BranchID: nil}
	if clientMatchesScope(scope, clientNoBranch) {
		t.Error("branch scope with nil BranchID must deny even a branchless client")
	}
}

// Block C: quality_control remains read-only at the authz layer even though its READ
// scope is now all-funnel. (HTTP ReadOnlyGuard + service IsReadOnly rely on this.)
func TestQualityControl_StaysReadOnly(t *testing.T) {
	if !authz.IsReadOnly(authz.RoleControl) {
		t.Error("quality_control must remain read-only after widening its read scope")
	}
	for _, roleID := range []int{authz.RoleSales, authz.RoleManagement, authz.RoleSystemAdmin, authz.RolePartner} {
		if authz.IsReadOnly(roleID) {
			t.Errorf("role %d must NOT be read-only", roleID)
		}
	}
}

// ─── cross-entity scope isolation ────────────────────────────────────────────

// TestLegalScopeLeadVsClient verifies that legal has All scope for clients but Forbidden for leads.
func TestLegalScopeLeadVsClient(t *testing.T) {
	leadScope, err := resolveLeadScope(1, authz.RoleLegal, nil)
	if err == nil || leadScope.Kind != ScopeKindForbidden {
		t.Errorf("legal must be Forbidden for leads, got kind=%v err=%v", leadScope.Kind, err)
	}

	clientScope, err := resolveClientScope(1, authz.RoleLegal, nil)
	if err != nil || clientScope.Kind != ScopeKindAll {
		t.Errorf("legal must be All for clients, got kind=%v err=%v", clientScope.Kind, err)
	}
}

// TestHRScopeAlwaysForbidden verifies HR is Forbidden for both leads and clients.
func TestHRScopeAlwaysForbidden(t *testing.T) {
	leadScope, err := resolveLeadScope(1, authz.RoleHR, nil)
	if err == nil || leadScope.Kind != ScopeKindForbidden {
		t.Errorf("hr must be Forbidden for leads, got kind=%v err=%v", leadScope.Kind, err)
	}

	clientScope, err := resolveClientScope(1, authz.RoleHR, nil)
	if err == nil || clientScope.Kind != ScopeKindForbidden {
		t.Errorf("hr must be Forbidden for clients, got kind=%v err=%v", clientScope.Kind, err)
	}
}

// TestPartnerScope_BranchLeadsAllClients verifies partner gets Branch scope for leads (dept-scoped)
// and All scope for clients (общая база).
func TestPartnerScope_BranchLeadsAllClients(t *testing.T) {
	const partnerID = 55
	branchID := 3
	userRepo := &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}

	leadScope, err := resolveLeadScope(partnerID, authz.RolePartner, userRepo)
	if err != nil || leadScope.Kind != ScopeKindBranch {
		t.Errorf("partner must be Branch-scoped for leads, got %+v err=%v", leadScope, err)
	}
	if leadScope.BranchID == nil || *leadScope.BranchID != branchID {
		t.Errorf("partner leads: expected branchID=%d, got %v", branchID, leadScope.BranchID)
	}

	clientScope, err := resolveClientScope(partnerID, authz.RolePartner, nil)
	if err != nil || clientScope.Kind != ScopeKindAll {
		t.Errorf("partner must be All (общая база) for clients, got %+v err=%v", clientScope, err)
	}
}

// TestQualityControlScopeIsAll verifies quality_control (RoleControl) is an all-funnel
// READ observer for both leads and clients (Block C). Write access stays blocked by
// ReadOnlyGuard + service IsReadOnly checks (verified in handler/service tests).
func TestQualityControlScopeIsAll(t *testing.T) {
	const qcUserID = 200

	leadScope, err := resolveLeadScope(qcUserID, authz.RoleControl, nil)
	if err != nil || leadScope.Kind != ScopeKindAll {
		t.Errorf("quality_control must be All for leads, got %+v err=%v", leadScope, err)
	}

	clientScope, err := resolveClientScope(qcUserID, authz.RoleControl, nil)
	if err != nil || clientScope.Kind != ScopeKindAll {
		t.Errorf("quality_control must be All for clients, got %+v err=%v", clientScope, err)
	}
}

// TestResolveUserBranch_NilRepoReturnsForbidden verifies that branch-scoped roles
// (sales/visa/partner) fail closed when userRepo is missing. quality_control is excluded —
// it is now ScopeKindAll and needs no branch lookup.
func TestResolveUserBranch_NilRepoReturnsForbidden(t *testing.T) {
	for _, roleID := range []int{authz.RoleSales, authz.RoleVisa, authz.RolePartner} {
		_, err := resolveLeadScope(1, roleID, nil)
		if err == nil {
			t.Errorf("role %d: expected error when userRepo is nil, got nil", roleID)
		}
	}
}
