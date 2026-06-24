package services

// Service-level tests for GetByID Own enforcement (ФАЗА 3a-verify).
//
// Pattern: resolveXxxScope + XxxMatchesScope combined = the exact enforcement
// executed inside getClientByIDWithScope (ScopeKindOwn branch) and
// LeadService.GetByID after the repo fetch.  No DB needed.

import (
	"errors"
	"testing"

	"turcompany/internal/authz"
	"turcompany/internal/models"
)

// ─── Client GetByID Own ───────────────────────────────────────────────────────

// По матрице партнёрский отдел видит ОБЩУЮ базу клиентов (ScopeKindAll):
// доступны и свои, и чужие клиенты.
func TestGetByID_Client_PartnerSeesAllClients(t *testing.T) {
	const partnerID = 7
	scope, err := resolveClientScope(partnerID, authz.RolePartner, nil)
	if err != nil {
		t.Fatalf("resolveClientScope: %v", err)
	}
	if scope.Kind != ScopeKindAll {
		t.Fatalf("partner must get ScopeKindAll (общая база), got %v", scope.Kind)
	}
	for _, ownerID := range []int{partnerID, 99} {
		client := &models.Client{ID: 1, OwnerID: ownerID}
		if !clientMatchesScope(scope, client) {
			t.Errorf("partner must access any client (owner_id=%d, общая база)", ownerID)
		}
	}
}

func TestGetByID_Client_LegalAccessesAnyClient(t *testing.T) {
	const legalUserID = 5
	scope, err := resolveClientScope(legalUserID, authz.RoleLegal, nil)
	if err != nil {
		t.Fatalf("resolveClientScope legal: %v", err)
	}
	if scope.Kind != ScopeKindAll {
		t.Fatalf("legal must get ScopeKindAll, got %v", scope.Kind)
	}
	for _, ownerID := range []int{legalUserID, 999, 1} {
		client := &models.Client{ID: 10, OwnerID: ownerID}
		if !clientMatchesScope(scope, client) {
			t.Errorf("legal must access any client (owner_id=%d)", ownerID)
		}
	}
}

func TestGetByID_Client_AdminManagementAccessAll(t *testing.T) {
	for _, roleID := range []int{authz.RoleSystemAdmin, authz.RoleManagement} {
		scope, err := resolveClientScope(1, roleID, nil)
		if err != nil {
			t.Errorf("role %d: unexpected error: %v", roleID, err)
			continue
		}
		if scope.Kind != ScopeKindAll {
			t.Errorf("role %d: expected ScopeKindAll, got %v", roleID, scope.Kind)
		}
		client := &models.Client{ID: 1, OwnerID: 999}
		if !clientMatchesScope(scope, client) {
			t.Errorf("role %d: must access any client", roleID)
		}
	}
}

func TestGetByID_Client_HRForbidden(t *testing.T) {
	_, err := resolveClientScope(1, authz.RoleHR, nil)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("hr client GetByID: want ErrForbidden, got %v", err)
	}
}

// ─── Lead GetByID Own ─────────────────────────────────────────────────────────

// По матрице партнёрский отдел видит лиды ТОЛЬКО своего отдела/филиала (ScopeKindBranch):
// лид своего филиала+отдела доступен, лид другого филиала — нет.
func TestGetByID_Lead_PartnerSeesOwnDepartmentLeads(t *testing.T) {
	const partnerID = 7
	branchID, deptID := 5, 3
	repo := &deptScopeUserRepoStub{
		user: &models.User{RoleID: authz.RolePartner, BranchID: &branchID, DepartmentID: &deptID},
	}
	scope, err := resolveLeadScope(partnerID, authz.RolePartner, repo)
	if err != nil {
		t.Fatalf("resolveLeadScope: %v", err)
	}
	if scope.Kind != ScopeKindBranch {
		t.Fatalf("partner must get ScopeKindBranch for leads, got %v", scope.Kind)
	}
	ownLead := &models.Leads{ID: 1, OwnerID: partnerID, BranchID: &branchID, DepartmentID: &deptID}
	if !leadMatchesScope(scope, ownLead) {
		t.Errorf("partner must access lead in own branch+department")
	}
	otherBranch := 6
	foreignLead := &models.Leads{ID: 2, OwnerID: 99, BranchID: &otherBranch, DepartmentID: &deptID}
	if leadMatchesScope(scope, foreignLead) {
		t.Errorf("partner must NOT access lead in a different branch")
	}
}

func TestGetByID_Lead_LegalForbidden(t *testing.T) {
	_, err := resolveLeadScope(1, authz.RoleLegal, nil)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("legal lead GetByID: want ErrForbidden, got %v", err)
	}
}

func TestGetByID_Lead_HRForbidden(t *testing.T) {
	_, err := resolveLeadScope(1, authz.RoleHR, nil)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("hr lead GetByID: want ErrForbidden, got %v", err)
	}
}

func TestGetByID_Lead_AdminManagementAccessAll(t *testing.T) {
	for _, roleID := range []int{authz.RoleSystemAdmin, authz.RoleManagement} {
		scope, err := resolveLeadScope(1, roleID, nil)
		if err != nil {
			t.Errorf("role %d: unexpected error: %v", roleID, err)
			continue
		}
		if scope.Kind != ScopeKindAll {
			t.Errorf("role %d: expected ScopeKindAll for leads, got %v", roleID, scope.Kind)
		}
		lead := &models.Leads{ID: 1, OwnerID: 999}
		if !leadMatchesScope(scope, lead) {
			t.Errorf("role %d: must access any lead", roleID)
		}
	}
}
