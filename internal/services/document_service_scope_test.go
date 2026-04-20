package services

import (
	"context"
	"testing"
	"time"

	"turcompany/internal/authz"
	"turcompany/internal/models"
)

type docScopeUserRepoStub struct {
	user *models.User
}

func (r *docScopeUserRepoStub) Create(*models.User) error                                { return nil }
func (r *docScopeUserRepoStub) GetByID(int) (*models.User, error)                        { return r.user, nil }
func (r *docScopeUserRepoStub) Update(*models.User) error                                { return nil }
func (r *docScopeUserRepoStub) Delete(int) error                                         { return nil }
func (r *docScopeUserRepoStub) List(int, int) ([]*models.User, error)                    { return nil, nil }
func (r *docScopeUserRepoStub) GetByEmail(string) (*models.User, error)                  { return nil, nil }
func (r *docScopeUserRepoStub) GetCount() (int, error)                                   { return 0, nil }
func (r *docScopeUserRepoStub) GetCountByRole(int) (int, error)                          { return 0, nil }
func (r *docScopeUserRepoStub) UpdatePassword(int, string) error                         { return nil }
func (r *docScopeUserRepoStub) UpdateRefresh(int, string, time.Time) error               { return nil }
func (r *docScopeUserRepoStub) RotateRefresh(string, string, time.Time) (*models.User, error) {
	return nil, nil
}
func (r *docScopeUserRepoStub) ClearRefresh(int) error                            { return nil }
func (r *docScopeUserRepoStub) GetByRefreshToken(string) (*models.User, error)   { return nil, nil }
func (r *docScopeUserRepoStub) VerifyUser(int) error                              { return nil }
func (r *docScopeUserRepoStub) UpdateTelegramLink(int, int64, bool) error         { return nil }
func (r *docScopeUserRepoStub) GetByIDSimple(int) (*models.User, error)           { return nil, nil }
func (r *docScopeUserRepoStub) GetTelegramSettings(context.Context, int64) (int64, bool, error) {
	return 0, false, nil
}
func (r *docScopeUserRepoStub) GetByChatID(context.Context, int64) (*models.User, error) {
	return nil, nil
}

func TestResolveListBranchScope_ScopedRolesIgnoreRequestedBranch(t *testing.T) {
	branchID := 2
	svc := &DocumentService{UserRepo: &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}}
	requested := int64(9)

	got, err := svc.ResolveListBranchScope(100, authz.RoleOperations, &requested)
	if err != nil {
		t.Fatalf("operations ResolveListBranchScope failed: %v", err)
	}
	if got == nil || *got != 2 {
		t.Fatalf("operations must be scoped to own branch, got %+v", got)
	}

	got, err = svc.ResolveListBranchScope(100, authz.RoleControl, &requested)
	if err != nil {
		t.Fatalf("control ResolveListBranchScope failed: %v", err)
	}
	if got == nil || *got != 2 {
		t.Fatalf("control must be scoped to own branch, got %+v", got)
	}
}

func TestResolveListBranchScope_AdminRoleKeepsRequestedBranch(t *testing.T) {
	svc := &DocumentService{}
	requested := int64(12)
	got, err := svc.ResolveListBranchScope(1, authz.RoleSystemAdmin, &requested)
	if err != nil {
		t.Fatalf("ResolveListBranchScope failed: %v", err)
	}
	if got == nil || *got != 12 {
		t.Fatalf("expected requested branch 12, got %+v", got)
	}
}

