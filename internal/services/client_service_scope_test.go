package services

import (
	"testing"

	"turcompany/internal/authz"
	"turcompany/internal/models"
)

func TestClientBranchScope_ControlUsesOwnBranch(t *testing.T) {
	branchID := 8
	svc := &ClientService{UserRepo: &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}}

	got, err := svc.branchScopeForRole(100, authz.RoleControl)
	if err != nil {
		t.Fatalf("branchScopeForRole failed: %v", err)
	}
	if got == nil || *got != branchID {
		t.Fatalf("control must be scoped to own branch, got %+v", got)
	}
}

func TestClientBranchScope_AdminKeepsGlobalScope(t *testing.T) {
	svc := &ClientService{}

	got, err := svc.branchScopeForRole(100, authz.RoleSystemAdmin)
	if err != nil {
		t.Fatalf("branchScopeForRole failed: %v", err)
	}
	if got != nil {
		t.Fatalf("system admin must keep global client scope, got %+v", got)
	}
}
