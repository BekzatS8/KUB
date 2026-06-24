package services

// ФАЗА 3b-2: unit-tests for department-scope logic in scope.go.

import (
	"context"
	"testing"
	"time"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

// ─── stub ────────────────────────────────────────────────────────────────────

type deptScopeUserRepoStub struct {
	user      *models.User
	deptByCode map[string]*int // code → id returned by GetDepartmentIDByCode
}

func (r *deptScopeUserRepoStub) GetByID(int) (*models.User, error) { return r.user, nil }
func (r *deptScopeUserRepoStub) GetDepartmentIDByCode(code string) (*int, error) {
	if r.deptByCode != nil {
		return r.deptByCode[code], nil
	}
	return nil, nil
}

// Remaining interface methods — not exercised by scope tests.
func (r *deptScopeUserRepoStub) Create(*models.User) error                   { return nil }
func (r *deptScopeUserRepoStub) ApplyUserPatch(int, *models.UserApprovalUpdatePayload) error {
	return nil
}
func (r *deptScopeUserRepoStub) Update(*models.User) error                   { return nil }
func (r *deptScopeUserRepoStub) Delete(int) error                            { return nil }
func (r *deptScopeUserRepoStub) List(int, int) ([]*models.User, error)       { return nil, nil }
func (r *deptScopeUserRepoStub) GetByEmail(string) (*models.User, error)     { return nil, nil }
func (r *deptScopeUserRepoStub) GetAuthByEmail(string) (*models.User, error) { return nil, nil }
func (r *deptScopeUserRepoStub) GetCount() (int, error)                      { return 0, nil }
func (r *deptScopeUserRepoStub) GetCountByRole(int) (int, error)             { return 0, nil }
func (r *deptScopeUserRepoStub) UpdatePassword(int, string) error            { return nil }
func (r *deptScopeUserRepoStub) UpdateRefresh(int, string, time.Time) error { return nil }
func (r *deptScopeUserRepoStub) RotateRefresh(string, string, time.Time) (*models.User, error) {
	return nil, nil
}
func (r *deptScopeUserRepoStub) ClearRefresh(int) error                         { return nil }
func (r *deptScopeUserRepoStub) GetByRefreshToken(string) (*models.User, error) { return nil, nil }
func (r *deptScopeUserRepoStub) VerifyUser(int) error                           { return nil }
func (r *deptScopeUserRepoStub) UpdateTelegramLink(int, int64, bool) error      { return nil }
func (r *deptScopeUserRepoStub) GetByIDSimple(int) (*models.User, error)        { return nil, nil }
func (r *deptScopeUserRepoStub) UpdateProfile(int, *models.User) error          { return nil }
func (r *deptScopeUserRepoStub) UpdateAvatar(int, string, string, string) error { return nil }
func (r *deptScopeUserRepoStub) UpdateAvatarCrop(int, *float64, *float64, *float64, *float64) error {
	return nil
}
func (r *deptScopeUserRepoStub) DeleteAvatar(int) error                   { return nil }
func (r *deptScopeUserRepoStub) GetTelegramSettings(context.Context, int64) (int64, bool, error) {
	return 0, false, nil
}
func (r *deptScopeUserRepoStub) GetByChatID(context.Context, int64) (*models.User, error) {
	return nil, nil
}

// ─── resolveUserContext ───────────────────────────────────────────────────────

func TestResolveUserContext_DeptFromUserField(t *testing.T) {
	branchID, deptID := 5, 3
	repo := &deptScopeUserRepoStub{
		user: &models.User{RoleID: authz.RoleSales, BranchID: &branchID, DepartmentID: &deptID},
	}
	gotBranch, gotDept, err := resolveUserContext(1, repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBranch == nil || *gotBranch != branchID {
		t.Errorf("branchID: want %d, got %v", branchID, gotBranch)
	}
	if gotDept == nil || *gotDept != deptID {
		t.Errorf("deptID: want %d, got %v", deptID, gotDept)
	}
}

func TestResolveUserContext_DeptFallbackByRoleCode(t *testing.T) {
	branchID := 5
	fallbackDeptID := 7
	repo := &deptScopeUserRepoStub{
		user:      &models.User{RoleID: authz.RoleSales, BranchID: &branchID}, // DepartmentID = nil
		deptByCode: map[string]*int{"sales": &fallbackDeptID},
	}
	_, gotDept, err := resolveUserContext(1, repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotDept == nil || *gotDept != fallbackDeptID {
		t.Errorf("deptID fallback: want %d, got %v", fallbackDeptID, gotDept)
	}
}

// ─── resolveLeadScope dept behaviour ─────────────────────────────────────────

func TestResolveLeadScope_Sales_HasDepartmentID(t *testing.T) {
	branchID, deptID := 10, 2
	repo := &deptScopeUserRepoStub{
		user: &models.User{RoleID: authz.RoleSales, BranchID: &branchID, DepartmentID: &deptID},
	}
	scope, err := resolveLeadScope(1, authz.RoleSales, repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope.DepartmentID == nil || *scope.DepartmentID != deptID {
		t.Errorf("sales scope.DepartmentID: want %d, got %v", deptID, scope.DepartmentID)
	}
}

func TestResolveLeadScope_Visa_HasDepartmentID(t *testing.T) {
	branchID, deptID := 10, 4
	repo := &deptScopeUserRepoStub{
		user: &models.User{RoleID: authz.RoleVisa, BranchID: &branchID, DepartmentID: &deptID},
	}
	scope, err := resolveLeadScope(1, authz.RoleVisa, repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope.DepartmentID == nil || *scope.DepartmentID != deptID {
		t.Errorf("visa scope.DepartmentID: want %d, got %v", deptID, scope.DepartmentID)
	}
}

func TestResolveLeadScope_QC_IsAllNoDepartmentID(t *testing.T) {
	branchID := 10
	repo := &deptScopeUserRepoStub{
		user: &models.User{RoleID: authz.RoleControl, BranchID: &branchID},
	}
	scope, err := resolveLeadScope(1, authz.RoleControl, repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Block C: qc observes all funnels — ScopeKindAll, no department restriction.
	if scope.Kind != ScopeKindAll {
		t.Errorf("qc scope.Kind: want All, got %v", scope.Kind)
	}
	if scope.DepartmentID != nil {
		t.Errorf("qc must have no DepartmentID, got %v", scope.DepartmentID)
	}
}

func TestResolveLeadScope_Management_IsAll(t *testing.T) {
	repo := &deptScopeUserRepoStub{user: &models.User{}}
	scope, err := resolveLeadScope(1, authz.RoleManagement, repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope.Kind != ScopeKindAll {
		t.Errorf("management scope.Kind: want All, got %v", scope.Kind)
	}
	if scope.DepartmentID != nil {
		t.Errorf("management must have no DepartmentID, got %v", scope.DepartmentID)
	}
}

func TestResolveLeadScope_Admin_IsAll(t *testing.T) {
	repo := &deptScopeUserRepoStub{user: &models.User{}}
	scope, err := resolveLeadScope(1, authz.RoleSystemAdmin, repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope.Kind != ScopeKindAll {
		t.Errorf("admin scope.Kind: want All, got %v", scope.Kind)
	}
	if scope.DepartmentID != nil {
		t.Errorf("admin must have no DepartmentID, got %v", scope.DepartmentID)
	}
}

// ─── leadMatchesScope with DepartmentID ──────────────────────────────────────

func intPtr(v int) *int { return &v }

func TestLeadMatchesScope_SalesSeesSameDeptAndBranch(t *testing.T) {
	scope := DataScope{Kind: ScopeKindBranch, BranchID: intPtr(1), DepartmentID: intPtr(2)}
	lead := &models.Leads{BranchID: intPtr(1), DepartmentID: intPtr(2)}
	if !leadMatchesScope(scope, lead) {
		t.Error("sales should see lead in own dept+branch")
	}
}

func TestLeadMatchesScope_SalesDoesNotSeeOtherDeptSameBranch(t *testing.T) {
	scope := DataScope{Kind: ScopeKindBranch, BranchID: intPtr(1), DepartmentID: intPtr(2)}
	lead := &models.Leads{BranchID: intPtr(1), DepartmentID: intPtr(9)}
	if leadMatchesScope(scope, lead) {
		t.Error("sales should NOT see lead in different dept (same branch)")
	}
}

func TestLeadMatchesScope_SalesDoesNotSeeOwnDeptDifferentBranch(t *testing.T) {
	scope := DataScope{Kind: ScopeKindBranch, BranchID: intPtr(1), DepartmentID: intPtr(2)}
	lead := &models.Leads{BranchID: intPtr(99), DepartmentID: intPtr(2)}
	if leadMatchesScope(scope, lead) {
		t.Error("sales should NOT see lead in different branch (same dept)")
	}
}

// Block A2 (fail-closed): a NULL-department lead is NOT visible to a department-scoped
// peer who does not own it, but IS visible to its owner.
func TestLeadMatchesScope_NullDepartmentHiddenFromNonOwnerPeer(t *testing.T) {
	scope := DataScope{Kind: ScopeKindBranch, BranchID: intPtr(1), DepartmentID: intPtr(2), UserID: 50}
	lead := &models.Leads{BranchID: intPtr(1), DepartmentID: nil, OwnerID: 99} // not owned by 50
	if leadMatchesScope(scope, lead) {
		t.Error("NULL-department lead must NOT leak to a department-scoped non-owner")
	}
}

func TestLeadMatchesScope_NullDepartmentVisibleToOwner(t *testing.T) {
	scope := DataScope{Kind: ScopeKindBranch, BranchID: intPtr(1), DepartmentID: intPtr(2), UserID: 50}
	lead := &models.Leads{BranchID: intPtr(1), DepartmentID: nil, OwnerID: 50} // owned by caller
	if !leadMatchesScope(scope, lead) {
		t.Error("NULL-department lead must remain visible to its owner")
	}
}

func TestLeadMatchesScope_NullDepartmentVisibleToBranchWideRole(t *testing.T) {
	// branch-wide role (no DepartmentID) — e.g. management/qc-as-All path uses ScopeKindAll,
	// but a Branch scope with nil DepartmentID must still see NULL-dept leads in branch.
	scope := DataScope{Kind: ScopeKindBranch, BranchID: intPtr(1)}
	lead := &models.Leads{BranchID: intPtr(1), DepartmentID: nil, OwnerID: 99}
	if !leadMatchesScope(scope, lead) {
		t.Error("branch-wide role (no department filter) must see NULL-department leads")
	}
}

func TestLeadMatchesScope_QC_SeesAllDeptsInBranch(t *testing.T) {
	scope := DataScope{Kind: ScopeKindBranch, BranchID: intPtr(1)} // no DepartmentID
	lead := &models.Leads{BranchID: intPtr(1), DepartmentID: intPtr(9)}
	if !leadMatchesScope(scope, lead) {
		t.Error("qc (no DepartmentID filter) should see any dept in own branch")
	}
}

// ─── dealMatchesScope with DepartmentID ──────────────────────────────────────

func TestDealMatchesScope_SalesSeesSameDeptAndBranch(t *testing.T) {
	scope := DataScope{Kind: ScopeKindBranch, BranchID: intPtr(1), DepartmentID: intPtr(2)}
	deal := &models.Deals{BranchID: intPtr(1), DepartmentID: intPtr(2)}
	if !dealMatchesScope(scope, deal) {
		t.Error("sales should see deal in own dept+branch")
	}
}

func TestDealMatchesScope_SalesDoesNotSeeOtherDeptSameBranch(t *testing.T) {
	scope := DataScope{Kind: ScopeKindBranch, BranchID: intPtr(1), DepartmentID: intPtr(2)}
	deal := &models.Deals{BranchID: intPtr(1), DepartmentID: intPtr(9)}
	if dealMatchesScope(scope, deal) {
		t.Error("sales should NOT see deal in different dept (same branch)")
	}
}

func TestDealMatchesScope_SalesDoesNotSeeOwnDeptDifferentBranch(t *testing.T) {
	scope := DataScope{Kind: ScopeKindBranch, BranchID: intPtr(1), DepartmentID: intPtr(2)}
	deal := &models.Deals{BranchID: intPtr(99), DepartmentID: intPtr(2)}
	if dealMatchesScope(scope, deal) {
		t.Error("sales should NOT see deal in different branch (same dept)")
	}
}

func TestDealMatchesScope_NullDepartmentVisibleWithinBranch(t *testing.T) {
	scope := DataScope{Kind: ScopeKindBranch, BranchID: intPtr(1), DepartmentID: intPtr(2)}
	deal := &models.Deals{BranchID: intPtr(1), DepartmentID: nil}
	if !dealMatchesScope(scope, deal) {
		t.Error("legacy deal with department_id=NULL must be visible within branch (soft fallback)")
	}
}

// ─── listLeadsForScope injects DepartmentID into filter ──────────────────────

type captureLeadRepo struct {
	capturedFilter repositories.LeadListFilter
}

func (r *captureLeadRepo) ListAllWithFilterAndArchiveScope(_ int, _ int, f repositories.LeadListFilter, _ repositories.ArchiveScope) ([]*models.Leads, error) {
	r.capturedFilter = f
	return nil, nil
}
func (r *captureLeadRepo) ListByOwnerWithFilterAndArchiveScope(_ int, _ int, _ int, f repositories.LeadListFilter, _ repositories.ArchiveScope) ([]*models.Leads, error) {
	r.capturedFilter = f
	return nil, nil
}
func (r *captureLeadRepo) CountAllWithFilterAndArchiveScope(f repositories.LeadListFilter, _ repositories.ArchiveScope) (int, error) {
	r.capturedFilter = f
	return 0, nil
}
func (r *captureLeadRepo) CountByOwnerWithFilterAndArchiveScope(_ int, f repositories.LeadListFilter, _ repositories.ArchiveScope) (int, error) {
	r.capturedFilter = f
	return 0, nil
}

func TestListLeadsForScope_BranchScope_InjectsDepartmentID(t *testing.T) {
	repo := &captureLeadRepo{}
	scope := DataScope{Kind: ScopeKindBranch, BranchID: intPtr(1), DepartmentID: intPtr(2)}
	_, _ = listLeadsForScope(repo, scope, 10, 0, repositories.LeadListFilter{}, repositories.ArchiveScopeActiveOnly)
	if repo.capturedFilter.DepartmentID == nil || *repo.capturedFilter.DepartmentID != 2 {
		t.Errorf("filter.DepartmentID: want 2, got %v", repo.capturedFilter.DepartmentID)
	}
}

func TestListLeadsForScope_AllScope_NoDepartmentID(t *testing.T) {
	repo := &captureLeadRepo{}
	scope := DataScope{Kind: ScopeKindAll}
	_, _ = listLeadsForScope(repo, scope, 10, 0, repositories.LeadListFilter{}, repositories.ArchiveScopeActiveOnly)
	if repo.capturedFilter.DepartmentID != nil {
		t.Errorf("All scope must not inject DepartmentID, got %v", repo.capturedFilter.DepartmentID)
	}
}
