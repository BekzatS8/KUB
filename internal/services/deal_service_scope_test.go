package services

// Characterizing tests for deal scope (ФАЗА 3b-1, Шаг 0 + Шаг 4).
//
// Part 1 (characterizing): lock the observable role→scope mapping that existed
// in the legacy branchScopeForRole.  These tests use nil Repo for forbidden
// paths (ErrForbidden is returned before any repo access) and must pass both
// before and after the refactor.
//
// Part 2 (routing): spy-repo tests for resolveDealScope / listDealsForScope /
// countDealsForScope (added during Step 1 of the refactor).

import (
	"errors"
	"testing"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

// ─── Part 1: characterizing — observable ListForRole behavior per role ─────

// TestDealListForRole_SalesHardGuard confirms sales cannot list all deals via
// ListForRole (sales accesses deals through ListMy*, same as leads/clients).
func TestDealListForRole_SalesHardGuard(t *testing.T) {
	svc := &DealService{} // nil Repo: ErrForbidden before repo access
	_, err := svc.ListForRole(1, authz.RoleSales, 10, 0, repositories.ArchiveScopeActiveOnly, repositories.DealListFilter{})
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("sales ListForRole hard-guard: want ErrForbidden, got %v", err)
	}
}

// TestDealListForRole_PartnerForbidden confirms partner cannot list deals at all.
func TestDealListForRole_PartnerForbidden(t *testing.T) {
	svc := &DealService{}
	_, err := svc.ListForRole(1, authz.RolePartner, 10, 0, repositories.ArchiveScopeActiveOnly, repositories.DealListFilter{})
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("partner ListForRole: want ErrForbidden, got %v", err)
	}
}

// TestDealListForRole_HRForbidden confirms hr cannot list deals.
func TestDealListForRole_HRForbidden(t *testing.T) {
	svc := &DealService{}
	_, err := svc.ListForRole(1, authz.RoleHR, 10, 0, repositories.ArchiveScopeActiveOnly, repositories.DealListFilter{})
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("hr ListForRole: want ErrForbidden, got %v", err)
	}
}

// TestDealListForRole_LegalForbidden confirms legal cannot list deals.
func TestDealListForRole_LegalForbidden(t *testing.T) {
	svc := &DealService{}
	_, err := svc.ListForRole(1, authz.RoleLegal, 10, 0, repositories.ArchiveScopeActiveOnly, repositories.DealListFilter{})
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("legal ListForRole: want ErrForbidden, got %v", err)
	}
}

// TestDealListForRole_UnknownRoleForbidden confirms unknown roles cannot list deals.
func TestDealListForRole_UnknownRoleForbidden(t *testing.T) {
	svc := &DealService{}
	_, err := svc.ListForRole(1, 999, 10, 0, repositories.ArchiveScopeActiveOnly, repositories.DealListFilter{})
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("unknown role ListForRole: want ErrForbidden, got %v", err)
	}
}

// ─── Part 1: resolveDealScope per role ───────────────────────────────────────

// TestResolveDealScope_AdminManagementAll confirms admin and management get ScopeKindAll.
func TestResolveDealScope_AdminManagementAll(t *testing.T) {
	for _, roleID := range []int{authz.RoleSystemAdmin, authz.RoleManagement} {
		scope, err := resolveDealScope(1, roleID, nil)
		if err != nil {
			t.Errorf("role %d: unexpected error: %v", roleID, err)
			continue
		}
		if scope.Kind != ScopeKindAll {
			t.Errorf("role %d: expected ScopeKindAll, got %v", roleID, scope.Kind)
		}
	}
}

// TestResolveDealScope_BranchRolesReturnBranch confirms sales/visa get ScopeKindBranch.
// (quality_control is now all-funnel — see TestResolveDealScope_ControlSeesAll.)
func TestResolveDealScope_BranchRolesReturnBranch(t *testing.T) {
	branchID := 5
	userRepo := &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}
	for _, roleID := range []int{authz.RoleSales, authz.RoleVisa} {
		scope, err := resolveDealScope(100, roleID, userRepo)
		if err != nil {
			t.Errorf("role %d: unexpected error: %v", roleID, err)
			continue
		}
		if scope.Kind != ScopeKindBranch {
			t.Errorf("role %d: expected ScopeKindBranch, got %v", roleID, scope.Kind)
		}
		if scope.BranchID == nil || *scope.BranchID != branchID {
			t.Errorf("role %d: expected branchID=%d, got %v", roleID, branchID, scope.BranchID)
		}
	}
}

// TestResolveDealScope_ControlSeesAll confirms quality_control observes all deals
// (Block C); no branch lookup needed, so a nil userRepo is fine.
func TestResolveDealScope_ControlSeesAll(t *testing.T) {
	scope, err := resolveDealScope(100, authz.RoleControl, nil)
	if err != nil {
		t.Fatalf("qc deal scope: unexpected error: %v", err)
	}
	if scope.Kind != ScopeKindAll {
		t.Fatalf("qc must observe all deals (ScopeKindAll), got %v", scope.Kind)
	}
}

// TestResolveDealScope_PartnerHRLegalForbidden confirms partner/hr/legal get ErrForbidden.
func TestResolveDealScope_PartnerHRLegalForbidden(t *testing.T) {
	for _, roleID := range []int{authz.RolePartner, authz.RoleHR, authz.RoleLegal, 999} {
		_, err := resolveDealScope(1, roleID, nil)
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("role %d: want ErrForbidden, got %v", roleID, err)
		}
	}
}

// ─── Part 2: routing tests with spy repo ─────────────────────────────────────

// dealRepoSpy records which branch filter (if any) was injected.
type dealRepoSpy struct {
	deals        []*models.Deals
	calledBranch *int
	calledAll    bool
}

func (r *dealRepoSpy) ListAllWithFilterAndArchiveScope(_, _ int, filter repositories.DealListFilter, _ repositories.ArchiveScope) ([]*models.Deals, error) {
	r.calledBranch = filter.BranchID
	if filter.BranchID == nil {
		r.calledAll = true
	}
	return r.deals, nil
}

func (r *dealRepoSpy) CountAllWithFilterAndArchiveScope(filter repositories.DealListFilter, _ repositories.ArchiveScope) (int, error) {
	r.calledBranch = filter.BranchID
	if filter.BranchID == nil {
		r.calledAll = true
	}
	return len(r.deals), nil
}

// TestListDealsForScope_BranchInjectsBranchID verifies Branch scope sets
// filter.BranchID before calling ListAll.
func TestListDealsForScope_BranchInjectsBranchID(t *testing.T) {
	branchID := 7
	spy := &dealRepoSpy{}
	scope := DataScope{Kind: ScopeKindBranch, BranchID: &branchID}

	_, err := listDealsForScope(spy, scope, 10, 0, repositories.DealListFilter{}, repositories.ArchiveScopeActiveOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.calledBranch == nil || *spy.calledBranch != branchID {
		t.Errorf("branch scope: expected filter.BranchID=%d, got %v", branchID, spy.calledBranch)
	}
}

// TestListDealsForScope_AllNoFilter verifies All scope calls ListAll with no branch filter.
func TestListDealsForScope_AllNoFilter(t *testing.T) {
	spy := &dealRepoSpy{}
	scope := DataScope{Kind: ScopeKindAll}

	_, err := listDealsForScope(spy, scope, 10, 0, repositories.DealListFilter{}, repositories.ArchiveScopeActiveOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spy.calledAll {
		t.Errorf("all scope: expected ListAll with no branch filter, got branchID=%v", spy.calledBranch)
	}
}

// TestCountDealsForScope_BranchInjectsBranchID verifies Branch scope sets
// filter.BranchID for the count query.
func TestCountDealsForScope_BranchInjectsBranchID(t *testing.T) {
	branchID := 3
	spy := &dealRepoSpy{deals: make([]*models.Deals, 4)}
	scope := DataScope{Kind: ScopeKindBranch, BranchID: &branchID}

	total, err := countDealsForScope(spy, scope, repositories.DealListFilter{}, repositories.ArchiveScopeActiveOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 4 {
		t.Errorf("expected count=4, got %d", total)
	}
	if spy.calledBranch == nil || *spy.calledBranch != branchID {
		t.Errorf("branch scope count: expected filter.BranchID=%d, got %v", branchID, spy.calledBranch)
	}
}

// TestCountDealsForScope_AllNoFilter verifies All scope count uses no branch filter.
func TestCountDealsForScope_AllNoFilter(t *testing.T) {
	spy := &dealRepoSpy{deals: make([]*models.Deals, 2)}
	scope := DataScope{Kind: ScopeKindAll}

	total, err := countDealsForScope(spy, scope, repositories.DealListFilter{}, repositories.ArchiveScopeActiveOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected count=2, got %d", total)
	}
	if !spy.calledAll {
		t.Errorf("all scope count: expected no branch filter, got branchID=%v", spy.calledBranch)
	}
}

// TestDealMatchesScope_AllAlwaysTrue verifies All scope matches any deal.
func TestDealMatchesScope_AllAlwaysTrue(t *testing.T) {
	scope := DataScope{Kind: ScopeKindAll}
	deal := &models.Deals{ID: 1}
	if !dealMatchesScope(scope, deal) {
		t.Errorf("All scope must match any deal")
	}
}

// TestDealMatchesScope_BranchMatchesOwnBranch verifies Branch scope matches same branch.
func TestDealMatchesScope_BranchMatchesOwnBranch(t *testing.T) {
	branchID := 9
	scope := DataScope{Kind: ScopeKindBranch, BranchID: &branchID}
	deal := &models.Deals{ID: 1, BranchID: &branchID}
	if !dealMatchesScope(scope, deal) {
		t.Errorf("Branch scope must match deal with same branch")
	}
}

// TestDealMatchesScope_BranchRejectsDifferentBranch verifies Branch scope blocks other branches.
func TestDealMatchesScope_BranchRejectsDifferentBranch(t *testing.T) {
	branchID := 9
	otherBranch := 5
	scope := DataScope{Kind: ScopeKindBranch, BranchID: &branchID}
	deal := &models.Deals{ID: 1, BranchID: &otherBranch}
	if dealMatchesScope(scope, deal) {
		t.Errorf("Branch scope must NOT match deal with different branch")
	}
}

// TestDealMatchesScope_ForbiddenAlwaysFalse verifies Forbidden scope never matches.
func TestDealMatchesScope_ForbiddenAlwaysFalse(t *testing.T) {
	scope := DataScope{Kind: ScopeKindForbidden}
	deal := &models.Deals{ID: 1}
	if dealMatchesScope(scope, deal) {
		t.Errorf("Forbidden scope must never match a deal")
	}
}
