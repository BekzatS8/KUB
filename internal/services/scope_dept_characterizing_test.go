package services

// Characterizing tests — ФАЗА 3b-2, Шаг 0.
//
// Lock current observable role→scope mapping before the department-scope
// change so regressions are caught immediately.  These tests must stay GREEN
// both before and after the change (scope kind must not change).

import (
	"testing"

	"turcompany/internal/authz"
	"turcompany/internal/models"
)

// TestCharacterize_SalesLeadScope_IsBranch confirms sales is Branch-scoped.
func TestCharacterize_SalesLeadScope_IsBranch(t *testing.T) {
	branchID := 7
	repo := &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}
	scope, err := resolveLeadScope(1, authz.RoleSales, repo)
	if err != nil {
		t.Fatalf("sales: unexpected error: %v", err)
	}
	if scope.Kind != ScopeKindBranch {
		t.Errorf("sales lead scope: want ScopeKindBranch, got %v", scope.Kind)
	}
	if scope.BranchID == nil || *scope.BranchID != branchID {
		t.Errorf("sales lead scope: want branchID=%d, got %v", branchID, scope.BranchID)
	}
}

// TestCharacterize_VisaLeadScope_IsBranch confirms visa is Branch-scoped.
func TestCharacterize_VisaLeadScope_IsBranch(t *testing.T) {
	branchID := 3
	repo := &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}
	scope, err := resolveLeadScope(1, authz.RoleVisa, repo)
	if err != nil {
		t.Fatalf("visa: unexpected error: %v", err)
	}
	if scope.Kind != ScopeKindBranch {
		t.Errorf("visa lead scope: want ScopeKindBranch, got %v", scope.Kind)
	}
	if scope.BranchID == nil || *scope.BranchID != branchID {
		t.Errorf("visa lead scope: want branchID=%d, got %v", branchID, scope.BranchID)
	}
}

// TestCharacterize_QCLeadScope_IsBranch confirms qc is Branch-scoped (read-only observer).
func TestCharacterize_QCLeadScope_IsBranch(t *testing.T) {
	branchID := 11
	repo := &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}
	scope, err := resolveLeadScope(1, authz.RoleControl, repo)
	if err != nil {
		t.Fatalf("qc: unexpected error: %v", err)
	}
	if scope.Kind != ScopeKindBranch {
		t.Errorf("qc lead scope: want ScopeKindBranch, got %v", scope.Kind)
	}
	if scope.BranchID == nil || *scope.BranchID != branchID {
		t.Errorf("qc lead scope: want branchID=%d, got %v", branchID, scope.BranchID)
	}
}

// TestCharacterize_SalesDealScope_IsBranch confirms sales deal scope is Branch.
func TestCharacterize_SalesDealScope_IsBranch(t *testing.T) {
	branchID := 5
	repo := &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}
	scope, err := resolveDealScope(1, authz.RoleSales, repo)
	if err != nil {
		t.Fatalf("sales deal: unexpected error: %v", err)
	}
	if scope.Kind != ScopeKindBranch {
		t.Errorf("sales deal scope: want ScopeKindBranch, got %v", scope.Kind)
	}
}

// TestCharacterize_QCDealScope_IsBranch confirms qc deal scope is Branch.
func TestCharacterize_QCDealScope_IsBranch(t *testing.T) {
	branchID := 9
	repo := &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}
	scope, err := resolveDealScope(1, authz.RoleControl, repo)
	if err != nil {
		t.Fatalf("qc deal: unexpected error: %v", err)
	}
	if scope.Kind != ScopeKindBranch {
		t.Errorf("qc deal scope: want ScopeKindBranch, got %v", scope.Kind)
	}
}
