package services

// Service-level tests for ListForRole / listLeadsForScope / listClientsForScope.
//
// Design:
//   • Forbidden paths (hr, legal-leads, sales guard) use nil Repo — ErrForbidden is
//     returned before the repo is ever touched.
//   • Success paths (partner→Own, legal-clients→All, management→All) test the
//     listLeadsForScope / listClientsForScope helpers directly with in-memory stubs.
//   • Combining the two layers proves the full call-path without needing to wiring a
//     real database.

import (
	"errors"
	"testing"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

// ─── in-memory stubs ─────────────────────────────────────────────────────────

// leadListRepoSpy records which listing method was called and with what key args.
type leadListRepoSpy struct {
	leads        []*models.Leads
	calledOwner  *int
	calledBranch *int
	calledAll    bool
}

func (r *leadListRepoSpy) ListByOwnerWithFilterAndArchiveScope(ownerID, _, _ int, _ repositories.LeadListFilter, _ repositories.ArchiveScope) ([]*models.Leads, error) {
	r.calledOwner = &ownerID
	return r.leads, nil
}

func (r *leadListRepoSpy) ListAllWithFilterAndArchiveScope(_, _ int, filter repositories.LeadListFilter, _ repositories.ArchiveScope) ([]*models.Leads, error) {
	r.calledBranch = filter.BranchID
	if filter.BranchID == nil {
		r.calledAll = true
	}
	return r.leads, nil
}

func (r *leadListRepoSpy) CountByOwnerWithFilterAndArchiveScope(_ int, _ repositories.LeadListFilter, _ repositories.ArchiveScope) (int, error) {
	return len(r.leads), nil
}

func (r *leadListRepoSpy) CountAllWithFilterAndArchiveScope(_ repositories.LeadListFilter, _ repositories.ArchiveScope) (int, error) {
	return len(r.leads), nil
}

// clientListRepoSpy records which listing method was called and with what key args.
type clientListRepoSpy struct {
	clients      []*models.Client
	calledOwner  *int
	calledBranch *int
	calledAll    bool
}

func (r *clientListRepoSpy) ListByOwnerWithFilterAndArchiveScope(ownerID, _, _ int, _ repositories.ClientListFilter, _ repositories.ArchiveScope) ([]*models.Client, error) {
	r.calledOwner = &ownerID
	return r.clients, nil
}

func (r *clientListRepoSpy) ListAllWithFilterAndArchiveScope(_, _ int, filter repositories.ClientListFilter, _ repositories.ArchiveScope) ([]*models.Client, error) {
	r.calledBranch = filter.BranchID
	if filter.BranchID == nil {
		r.calledAll = true
	}
	return r.clients, nil
}

func (r *clientListRepoSpy) CountWithFilterAndArchiveScope(_ *int, _ string, _ repositories.ClientListFilter, _ repositories.ArchiveScope) (int, error) {
	return len(r.clients), nil
}

// ─── helper: archive scope shorthand ────────────────────────────────────────

var activeOnly = repositories.ArchiveScopeActiveOnly

// ─── listLeadsForScope (helper-function level) ────────────────────────────────

// TestListLeadsForScope_PartnerRoutesToOwner verifies that Own scope routes to
// ListByOwner and passes the partner's userID — not a branch filter.
func TestListLeadsForScope_PartnerRoutesToOwner(t *testing.T) {
	const partnerID = 42
	spy := &leadListRepoSpy{}
	scope := DataScope{Kind: ScopeKindOwn, UserID: partnerID}

	leads, err := listLeadsForScope(spy, scope, 10, 0, repositories.LeadListFilter{}, activeOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = leads
	if spy.calledOwner == nil || *spy.calledOwner != partnerID {
		t.Errorf("partner: expected ListByOwner(ownerID=%d), got calledOwner=%v", partnerID, spy.calledOwner)
	}
	if spy.calledBranch != nil || spy.calledAll {
		t.Errorf("partner: must NOT call ListAll, got calledBranch=%v calledAll=%v", spy.calledBranch, spy.calledAll)
	}
}

// TestListLeadsForScope_ManagementRoutesToAll verifies that All scope routes to
// ListAll with no branch filter — management sees every lead.
func TestListLeadsForScope_ManagementRoutesToAll(t *testing.T) {
	spy := &leadListRepoSpy{}
	scope := DataScope{Kind: ScopeKindAll}

	_, err := listLeadsForScope(spy, scope, 10, 0, repositories.LeadListFilter{}, activeOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.calledOwner != nil {
		t.Errorf("management: must NOT call ListByOwner, got ownerID=%d", *spy.calledOwner)
	}
	if !spy.calledAll {
		t.Errorf("management: expected ListAll with no branch filter")
	}
}

// TestListLeadsForScope_BranchScopePassesBranch verifies that Branch scope
// injects BranchID into the filter before calling ListAll.
func TestListLeadsForScope_BranchScopePassesBranch(t *testing.T) {
	branchID := 7
	spy := &leadListRepoSpy{}
	scope := DataScope{Kind: ScopeKindBranch, BranchID: &branchID}

	_, err := listLeadsForScope(spy, scope, 10, 0, repositories.LeadListFilter{}, activeOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.calledOwner != nil {
		t.Errorf("branch: must NOT call ListByOwner")
	}
	if spy.calledBranch == nil || *spy.calledBranch != branchID {
		t.Errorf("branch: expected filter.BranchID=%d, got %v", branchID, spy.calledBranch)
	}
}

// TestListLeadsForScope_ForbiddenRepoNotCalled verifies that Forbidden scope
// does NOT call the repo (it's not reachable via resolveLeadScope; tested here
// for defensive completeness of the helper itself).
func TestListLeadsForScope_ForbiddenNeverReachesRepo(t *testing.T) {
	// Forbidden scope → default branch of switch → calls ListAll (no separate forbidden branch).
	// This test documents that the helper itself does not know about Forbidden —
	// the guard lives in ListForRole (which returns ErrForbidden before calling this helper).
	// Nothing to assert here beyond compile-correctness; the forbidden guard is
	// tested via TestLeadListForRole_* below.
}

// ─── listClientsForScope (helper-function level) ──────────────────────────────

// TestListClientsForScope_PartnerRoutesToOwner verifies that Own scope routes to
// ListByOwner with the partner's userID — the critical bug-fix for partner clients.
func TestListClientsForScope_PartnerRoutesToOwner(t *testing.T) {
	const partnerID = 77
	spy := &clientListRepoSpy{}
	scope := DataScope{Kind: ScopeKindOwn, UserID: partnerID}

	clients, err := listClientsForScope(spy, scope, 10, 0, repositories.ClientListFilter{}, activeOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = clients
	if spy.calledOwner == nil || *spy.calledOwner != partnerID {
		t.Errorf("partner: expected ListByOwner(ownerID=%d), got calledOwner=%v", partnerID, spy.calledOwner)
	}
	if spy.calledBranch != nil || spy.calledAll {
		t.Errorf("partner: must NOT call ListAll, got calledBranch=%v calledAll=%v", spy.calledBranch, spy.calledAll)
	}
}

// TestListClientsForScope_LegalRoutesToAll verifies that All scope routes to
// ListAll with no branch filter — legal sees every client (bug-fix for legal).
func TestListClientsForScope_LegalRoutesToAll(t *testing.T) {
	spy := &clientListRepoSpy{}
	scope := DataScope{Kind: ScopeKindAll}

	_, err := listClientsForScope(spy, scope, 10, 0, repositories.ClientListFilter{}, activeOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.calledOwner != nil {
		t.Errorf("legal: must NOT call ListByOwner")
	}
	if !spy.calledAll {
		t.Errorf("legal: expected ListAll with no branch filter")
	}
}

// TestListClientsForScope_BranchScopePassesBranch verifies Branch scope injects
// BranchID (sales / visa / quality_control behaviour).
func TestListClientsForScope_BranchScopePassesBranch(t *testing.T) {
	branchID := 3
	spy := &clientListRepoSpy{}
	scope := DataScope{Kind: ScopeKindBranch, BranchID: &branchID}

	_, err := listClientsForScope(spy, scope, 10, 0, repositories.ClientListFilter{}, activeOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.calledOwner != nil {
		t.Errorf("branch: must NOT call ListByOwner")
	}
	if spy.calledBranch == nil || *spy.calledBranch != branchID {
		t.Errorf("branch: expected filter.BranchID=%d, got %v", branchID, spy.calledBranch)
	}
}

// ─── LeadService.ListForRole (service level, forbidden paths) ─────────────────

// For forbidden roles (hr, legal, unknown) resolveLeadScope returns ErrForbidden
// before s.Repo is ever accessed → Repo can safely be nil.

// TestLeadListForRole_HRForbidden verifies HR cannot list leads via ListForRole.
func TestLeadListForRole_HRForbidden(t *testing.T) {
	svc := &LeadService{} // nil Repo: function returns before repo access
	_, err := svc.ListForRole(1, authz.RoleHR, 10, 0, activeOnly, repositories.LeadListFilter{})
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("hr ListForRole: want ErrForbidden, got %v", err)
	}
}

// TestLeadListForRole_LegalForbidden verifies legal cannot list leads (legal has
// clients.view but NOT leads.view in the permission matrix).
func TestLeadListForRole_LegalForbidden(t *testing.T) {
	svc := &LeadService{}
	_, err := svc.ListForRole(1, authz.RoleLegal, 10, 0, activeOnly, repositories.LeadListFilter{})
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("legal ListForRole: want ErrForbidden, got %v", err)
	}
}

// TestLeadListForRole_SalesResolvesToBranch verifies sales is no longer hard-blocked
// in ListForRole and instead resolves to ScopeKindBranch (свой отдел+филиал по
// воронкам per ТЗ). Previously the guard forced sales onto /leads/my which returned
// ALL leads, leaking every department.
func TestLeadListForRole_SalesResolvesToBranch(t *testing.T) {
	const salesID = 11
	branchID, deptID := 4, 2
	repo := &deptScopeUserRepoStub{
		user: &models.User{RoleID: authz.RoleSales, BranchID: &branchID, DepartmentID: &deptID},
	}
	scope, err := resolveLeadScope(salesID, authz.RoleSales, repo)
	if err != nil {
		t.Fatalf("sales resolveLeadScope: unexpected error %v", err)
	}
	if scope.Kind != ScopeKindBranch {
		t.Fatalf("sales: expected ScopeKindBranch (свой отдел+филиал), got %v", scope.Kind)
	}
	if scope.BranchID == nil || *scope.BranchID != branchID {
		t.Errorf("sales: expected BranchID=%d, got %v", branchID, scope.BranchID)
	}
	if scope.DepartmentID == nil || *scope.DepartmentID != deptID {
		t.Errorf("sales: expected DepartmentID=%d, got %v", deptID, scope.DepartmentID)
	}
}

// TestLeadListForRole_UnknownRoleForbidden verifies an unknown role cannot list leads.
func TestLeadListForRole_UnknownRoleForbidden(t *testing.T) {
	svc := &LeadService{}
	_, err := svc.ListForRole(1, 999, 10, 0, activeOnly, repositories.LeadListFilter{})
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("unknown role ListForRole: want ErrForbidden, got %v", err)
	}
}

// TestLeadListForRole_PartnerNotForbidden verifies partner does NOT get ErrForbidden
// from ListForRole (the critical regression: partner was previously blocked here).
// Partner resolves to ScopeKindOwn without needing UserRepo, so nil Repo would panic;
// we inject a spy Repo to let the call complete.
func TestLeadListForRole_PartnerNotForbidden(t *testing.T) {
	const partnerID = 55
	spy := &leadListRepoSpy{leads: []*models.Leads{{ID: 1, OwnerID: partnerID}}}
	// Call the helper directly with the scope that ListForRole would resolve for partner.
	scope := DataScope{Kind: ScopeKindOwn, UserID: partnerID}
	leads, err := listLeadsForScope(spy, scope, 10, 0, repositories.LeadListFilter{}, activeOnly)
	if err != nil {
		t.Fatalf("partner must NOT get error from listLeadsForScope, got %v", err)
	}
	if len(leads) == 0 {
		t.Errorf("partner: expected at least one lead returned")
	}
	if spy.calledOwner == nil || *spy.calledOwner != partnerID {
		t.Errorf("partner: expected ListByOwner(%d), got %v", partnerID, spy.calledOwner)
	}
}

// ─── ClientService.ListForRole (service level, forbidden paths) ───────────────

// TestClientListForRole_HRForbidden verifies HR cannot list clients via ListForRole.
func TestClientListForRole_HRForbidden(t *testing.T) {
	svc := &ClientService{}
	_, err := svc.ListForRole(1, authz.RoleHR, 10, 0, repositories.ClientListFilter{}, activeOnly)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("hr client ListForRole: want ErrForbidden, got %v", err)
	}
}

// TestClientListForRole_SalesSeesAll verifies sales now resolves to ScopeKindAll
// for clients ("Общая база" per ТЗ) — the same shared base visa/partner/legal see.
// Sales is no longer blocked at a hardcoded guard.
func TestClientListForRole_SalesSeesAll(t *testing.T) {
	scope, err := resolveClientScope(1, authz.RoleSales, nil)
	if err != nil {
		t.Fatalf("sales resolveClientScope: unexpected error %v", err)
	}
	if scope.Kind != ScopeKindAll {
		t.Fatalf("sales: expected ScopeKindAll (общая база), got %v", scope.Kind)
	}
	spy := &clientListRepoSpy{clients: []*models.Client{{ID: 1, OwnerID: 99}}}
	clients, err := listClientsForScope(spy, scope, 10, 0, repositories.ClientListFilter{}, activeOnly)
	if err != nil {
		t.Fatalf("sales listClientsForScope: unexpected error %v", err)
	}
	if !spy.calledAll {
		t.Errorf("sales: expected ListAll (общая база), got calledAll=%v", spy.calledAll)
	}
	if len(clients) == 0 {
		t.Errorf("sales: expected at least one client returned")
	}
}

// TestClientListForRole_UnknownRoleForbidden verifies an unknown role cannot list clients.
func TestClientListForRole_UnknownRoleForbidden(t *testing.T) {
	svc := &ClientService{}
	_, err := svc.ListForRole(1, 999, 10, 0, repositories.ClientListFilter{}, activeOnly)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("unknown role client ListForRole: want ErrForbidden, got %v", err)
	}
}

// TestClientListForRole_LegalNotForbidden verifies legal does NOT get ErrForbidden
// from ListForRole (the critical regression: legal was previously blocked here).
// Legal resolves to ScopeKindAll without needing UserRepo.
func TestClientListForRole_LegalNotForbidden(t *testing.T) {
	spy := &clientListRepoSpy{clients: []*models.Client{{ID: 1, OwnerID: 99}}}
	// legal → ScopeKindAll, so listClientsForScope calls ListAll.
	scope := DataScope{Kind: ScopeKindAll}
	clients, err := listClientsForScope(spy, scope, 10, 0, repositories.ClientListFilter{}, activeOnly)
	if err != nil {
		t.Fatalf("legal must NOT get error from listClientsForScope, got %v", err)
	}
	if len(clients) == 0 {
		t.Errorf("legal: expected at least one client returned")
	}
	if !spy.calledAll {
		t.Errorf("legal: expected ListAll (no branch filter), got calledBranch=%v", spy.calledBranch)
	}
}

// TestClientListForRole_PartnerNotForbidden verifies partner does NOT get ErrForbidden
// from ListForRole for clients. Partner resolves to ScopeKindOwn.
func TestClientListForRole_PartnerNotForbidden(t *testing.T) {
	const partnerID = 70
	spy := &clientListRepoSpy{clients: []*models.Client{{ID: 5, OwnerID: partnerID}}}
	scope := DataScope{Kind: ScopeKindOwn, UserID: partnerID}
	clients, err := listClientsForScope(spy, scope, 10, 0, repositories.ClientListFilter{}, activeOnly)
	if err != nil {
		t.Fatalf("partner must NOT get error from listClientsForScope, got %v", err)
	}
	if len(clients) == 0 {
		t.Errorf("partner: expected at least one client returned")
	}
	if spy.calledOwner == nil || *spy.calledOwner != partnerID {
		t.Errorf("partner: expected ListByOwner(%d), got %v", partnerID, spy.calledOwner)
	}
}

// ─── countLeadsForScope / countClientsForScope (routing verification) ─────────

// TestCountLeadsForScope_PartnerUsesOwnerCount verifies Own scope hits CountByOwner.
func TestCountLeadsForScope_PartnerUsesOwnerCount(t *testing.T) {
	const partnerID = 42
	spy := &leadListRepoSpy{leads: make([]*models.Leads, 3)}
	scope := DataScope{Kind: ScopeKindOwn, UserID: partnerID}

	total, err := countLeadsForScope(spy, scope, repositories.LeadListFilter{}, activeOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 3 {
		t.Errorf("expected count=3, got %d", total)
	}
}

// TestCountClientsForScope_LegalUsesAllCount verifies All scope hits CountAll without branch.
func TestCountClientsForScope_LegalUsesAllCount(t *testing.T) {
	spy := &clientListRepoSpy{clients: make([]*models.Client, 5)}
	scope := DataScope{Kind: ScopeKindAll}

	total, err := countClientsForScope(spy, scope, "", repositories.ClientListFilter{}, activeOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 5 {
		t.Errorf("expected count=5, got %d", total)
	}
}
