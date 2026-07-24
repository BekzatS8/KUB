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

// TestCharacterize_SalesLeadScope_IsAll confirms sales sees all leads
// (общий пул для создания сделок, обратная связь 24.07.2026).
func TestCharacterize_SalesLeadScope_IsAll(t *testing.T) {
	branchID := 7
	repo := &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}
	scope, err := resolveLeadScope(1, authz.RoleSales, repo)
	if err != nil {
		t.Fatalf("sales: unexpected error: %v", err)
	}
	if scope.Kind != ScopeKindAll {
		t.Errorf("sales lead scope: want ScopeKindAll, got %v", scope.Kind)
	}
}

// TestCharacterize_VisaLeadScope_IsAll confirms visa sees all leads.
func TestCharacterize_VisaLeadScope_IsAll(t *testing.T) {
	branchID := 3
	repo := &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}
	scope, err := resolveLeadScope(1, authz.RoleVisa, repo)
	if err != nil {
		t.Fatalf("visa: unexpected error: %v", err)
	}
	if scope.Kind != ScopeKindAll {
		t.Errorf("visa lead scope: want ScopeKindAll, got %v", scope.Kind)
	}
}

// TestCharacterize_QCLeadScope_IsAll confirms qc is an all-funnel read observer
// (Block C: widened from branch to all; read-only enforced separately).
func TestCharacterize_QCLeadScope_IsAll(t *testing.T) {
	branchID := 11
	repo := &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}
	scope, err := resolveLeadScope(1, authz.RoleControl, repo)
	if err != nil {
		t.Fatalf("qc: unexpected error: %v", err)
	}
	if scope.Kind != ScopeKindAll {
		t.Errorf("qc lead scope: want ScopeKindAll, got %v", scope.Kind)
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

// TestCharacterize_QCDealScope_IsAll confirms qc deal scope is all-funnel (Block C).
func TestCharacterize_QCDealScope_IsAll(t *testing.T) {
	branchID := 9
	repo := &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}
	scope, err := resolveDealScope(1, authz.RoleControl, repo)
	if err != nil {
		t.Fatalf("qc deal: unexpected error: %v", err)
	}
	if scope.Kind != ScopeKindAll {
		t.Errorf("qc deal scope: want ScopeKindAll, got %v", scope.Kind)
	}
}
