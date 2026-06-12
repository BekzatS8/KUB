package services

import (
	"testing"

	"turcompany/internal/authz"
	"turcompany/internal/models"
)

func TestClientBranchScope_ControlUsesOwnBranch(t *testing.T) {
	branchID := 8
	userRepo := &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}

	scope, err := resolveClientScope(100, authz.RoleControl, userRepo)
	if err != nil {
		t.Fatalf("resolveClientScope failed: %v", err)
	}
	if scope.Kind != ScopeKindBranch || scope.BranchID == nil || *scope.BranchID != branchID {
		t.Fatalf("control must be scoped to own branch, got %+v", scope)
	}
}

func TestClientBranchScope_AdminKeepsGlobalScope(t *testing.T) {
	scope, err := resolveClientScope(100, authz.RoleSystemAdmin, nil)
	if err != nil {
		t.Fatalf("resolveClientScope failed: %v", err)
	}
	if scope.Kind != ScopeKindAll {
		t.Fatalf("system admin must have global client scope, got %+v", scope)
	}
}
