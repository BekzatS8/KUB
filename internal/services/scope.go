package services

import (
	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

// ScopeKind describes the kind of data-access restriction applied to an entity.
type ScopeKind int

const (
	ScopeKindAll      ScopeKind = iota // unrestricted: all records visible
	ScopeKindBranch                    // restrict to branch_id = user's branch
	ScopeKindOwn                       // restrict to owner_id = userID
	ScopeKindForbidden                 // no access at all
)

// DataScope carries the resolved access restriction for a single entity type.
type DataScope struct {
	Kind         ScopeKind
	BranchID     *int // set when Kind == ScopeKindBranch
	DepartmentID *int // set for sales/visa (branch+dept combined filter)
	UserID       int  // set when Kind == ScopeKindOwn
}

// roleDeptCode maps role IDs that carry an implicit department to that
// department's code. Used as fallback when users.department_id is NULL.
var roleDeptCode = map[int]string{
	authz.RoleSales: "sales",
	authz.RoleVisa:  "visa",
}

// resolveUserContext fetches branch and department for userID.
// Department is read from users.department_id; if NULL, falls back to the
// department whose code matches the role code.
func resolveUserContext(userID int, userRepo repositories.UserRepository) (branchID *int, deptID *int, err error) {
	if userRepo == nil {
		return nil, nil, ErrForbidden
	}
	u, err := userRepo.GetByID(userID)
	if err != nil || u == nil || u.BranchID == nil {
		return nil, nil, ErrForbidden
	}
	branchID = u.BranchID
	if u.DepartmentID != nil {
		return branchID, u.DepartmentID, nil
	}
	if code, ok := roleDeptCode[u.RoleID]; ok {
		deptID, err = userRepo.GetDepartmentIDByCode(code)
		if err != nil {
			return nil, nil, err
		}
	}
	return branchID, deptID, nil
}

// resolveUserBranch fetches the branch for userID.
func resolveUserBranch(userID int, userRepo repositories.UserRepository) (*int, error) {
	if userRepo == nil {
		return nil, ErrForbidden
	}
	u, err := userRepo.GetByID(userID)
	if err != nil || u == nil || u.BranchID == nil {
		return nil, ErrForbidden
	}
	return u.BranchID, nil
}

// resolveLeadScope returns the DataScope for the leads entity.
//
// LEADS mapping (from permission matrix + ТЗ №2/№4):
//   admin / management            → All
//   sales / visa / quality_control → Branch(user.BranchID)
//   partner                        → Own(userID)
//   hr / legal / unknown           → Forbidden
func resolveLeadScope(userID, roleID int, userRepo repositories.UserRepository) (DataScope, error) {
	switch roleID {
	case authz.RoleManagement, authz.RoleSystemAdmin, authz.RoleControl:
		// quality_control is an all-funnel READ observer across all departments/branches.
		// Read-only is enforced separately (ReadOnlyGuard + service IsReadOnly checks).
		return DataScope{Kind: ScopeKindAll}, nil
	case authz.RoleSales, authz.RoleVisa:
		branchID, deptID, err := resolveUserContext(userID, userRepo)
		if err != nil {
			return DataScope{Kind: ScopeKindForbidden}, err
		}
		// UserID is carried so a department-scoped role can still see leads it owns
		// (incl. department_id IS NULL ones) without leaking NULL-dept leads to peers.
		return DataScope{Kind: ScopeKindBranch, BranchID: branchID, DepartmentID: deptID, UserID: userID}, nil
	case authz.RolePartner:
		return DataScope{Kind: ScopeKindOwn, UserID: userID}, nil
	default:
		return DataScope{Kind: ScopeKindForbidden}, ErrForbidden
	}
}

// resolveClientScope returns the DataScope for the clients entity.
//
// CLIENTS mapping (from permission matrix + ТЗ №2/№4):
//   admin / management / sales / visa / legal → All (общая база)
//   quality_control                            → All (read-only enforced elsewhere)
//   partner                                    → Own(userID) — видит только своих клиентов
//   hr / unknown                               → Forbidden
func resolveClientScope(userID, roleID int, userRepo repositories.UserRepository) (DataScope, error) {
	switch roleID {
	case authz.RolePartner:
		return DataScope{Kind: ScopeKindOwn, UserID: userID}, nil
	case authz.RoleSales, authz.RoleVisa,
		authz.RoleManagement, authz.RoleSystemAdmin, authz.RoleLegal, authz.RoleControl:
		// quality_control observes all clients (read-only enforced elsewhere).
		return DataScope{Kind: ScopeKindAll}, nil
	default:
		return DataScope{Kind: ScopeKindForbidden}, ErrForbidden
	}
}

// ─── Repo interfaces for scope-based listing ─────────────────────────────────

// leadListRepo is the minimal interface covering the listing methods of
// LeadRepository that are needed for scope-based access control.
// *repositories.LeadRepository satisfies this interface (duck typing).
type leadListRepo interface {
	ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset int, filter repositories.LeadListFilter, scope repositories.ArchiveScope) ([]*models.Leads, error)
	ListAllWithFilterAndArchiveScope(limit, offset int, filter repositories.LeadListFilter, scope repositories.ArchiveScope) ([]*models.Leads, error)
	CountByOwnerWithFilterAndArchiveScope(ownerID int, filter repositories.LeadListFilter, scope repositories.ArchiveScope) (int, error)
	CountAllWithFilterAndArchiveScope(filter repositories.LeadListFilter, scope repositories.ArchiveScope) (int, error)
}

// clientListRepo is the minimal interface covering the listing methods of
// ClientRepository that are needed for scope-based access control.
// *repositories.ClientRepository satisfies this interface (duck typing).
type clientListRepo interface {
	ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset int, filter repositories.ClientListFilter, scope repositories.ArchiveScope) ([]*models.Client, error)
	ListAllWithFilterAndArchiveScope(limit, offset int, filter repositories.ClientListFilter, scope repositories.ArchiveScope) ([]*models.Client, error)
	CountWithFilterAndArchiveScope(ownerID *int, forcedType string, filter repositories.ClientListFilter, scope repositories.ArchiveScope) (int, error)
}

// ─── Scope-based routing helpers ─────────────────────────────────────────────

// listLeadsForScope executes the appropriate repository call based on the
// resolved DataScope. Branch and owner filters are applied here so that every
// call-site stays in sync.
func listLeadsForScope(repo leadListRepo, scope DataScope, limit, offset int, filter repositories.LeadListFilter, archiveScope repositories.ArchiveScope) ([]*models.Leads, error) {
	switch scope.Kind {
	case ScopeKindOwn:
		return repo.ListByOwnerWithFilterAndArchiveScope(scope.UserID, limit, offset, filter, archiveScope)
	case ScopeKindBranch:
		filter.BranchID = scope.BranchID
		filter.DepartmentID = scope.DepartmentID
		filter.ScopeUserID = scopeOwnerForDept(scope)
		return repo.ListAllWithFilterAndArchiveScope(limit, offset, filter, archiveScope)
	default: // ScopeKindAll
		return repo.ListAllWithFilterAndArchiveScope(limit, offset, filter, archiveScope)
	}
}

// scopeOwnerForDept returns the owning userID to widen a department-scoped lead
// query with "OR (department_id IS NULL AND owner_id = userID)" — so the owner
// still sees their own NULL-department leads without leaking them to peers.
// Returns nil when the scope carries no department filter or no user.
func scopeOwnerForDept(scope DataScope) *int {
	if scope.DepartmentID == nil || scope.UserID == 0 {
		return nil
	}
	uid := scope.UserID
	return &uid
}

// countLeadsForScope executes the appropriate count query based on the
// resolved DataScope.
func countLeadsForScope(repo leadListRepo, scope DataScope, filter repositories.LeadListFilter, archiveScope repositories.ArchiveScope) (int, error) {
	switch scope.Kind {
	case ScopeKindOwn:
		return repo.CountByOwnerWithFilterAndArchiveScope(scope.UserID, filter, archiveScope)
	case ScopeKindBranch:
		filter.BranchID = scope.BranchID
		filter.DepartmentID = scope.DepartmentID
		filter.ScopeUserID = scopeOwnerForDept(scope)
		return repo.CountAllWithFilterAndArchiveScope(filter, archiveScope)
	default: // ScopeKindAll
		return repo.CountAllWithFilterAndArchiveScope(filter, archiveScope)
	}
}

// listClientsForScope executes the appropriate repository call based on the
// resolved DataScope. Callers must set filter.ClientType before calling when
// they need type-filtered results (e.g. individuals / companies).
func listClientsForScope(repo clientListRepo, scope DataScope, limit, offset int, filter repositories.ClientListFilter, archiveScope repositories.ArchiveScope) ([]*models.Client, error) {
	switch scope.Kind {
	case ScopeKindOwn:
		return repo.ListByOwnerWithFilterAndArchiveScope(scope.UserID, limit, offset, filter, archiveScope)
	case ScopeKindBranch:
		filter.BranchID = scope.BranchID
		return repo.ListAllWithFilterAndArchiveScope(limit, offset, filter, archiveScope)
	default: // ScopeKindAll
		return repo.ListAllWithFilterAndArchiveScope(limit, offset, filter, archiveScope)
	}
}

// countClientsForScope executes the appropriate count query. forcedType (e.g.
// models.ClientTypeIndividual) is forwarded to the repository as-is; pass ""
// for all types.
func countClientsForScope(repo clientListRepo, scope DataScope, forcedType string, filter repositories.ClientListFilter, archiveScope repositories.ArchiveScope) (int, error) {
	switch scope.Kind {
	case ScopeKindOwn:
		ownerID := scope.UserID
		return repo.CountWithFilterAndArchiveScope(&ownerID, forcedType, filter, archiveScope)
	case ScopeKindBranch:
		filter.BranchID = scope.BranchID
		return repo.CountWithFilterAndArchiveScope(nil, forcedType, filter, archiveScope)
	default: // ScopeKindAll
		return repo.CountWithFilterAndArchiveScope(nil, forcedType, filter, archiveScope)
	}
}

// leadMatchesScope returns true when lead is accessible under the given scope.
func leadMatchesScope(scope DataScope, lead *models.Leads) bool {
	if lead == nil {
		return false
	}
	switch scope.Kind {
	case ScopeKindAll:
		return true
	case ScopeKindOwn:
		return lead.OwnerID == scope.UserID
	case ScopeKindBranch:
		if scope.BranchID != nil {
			if lead.BranchID == nil || *lead.BranchID != *scope.BranchID {
				return false
			}
		}
		if scope.DepartmentID != nil {
			// fail-closed: a department-scoped role sees a lead only if it belongs to
			// that department, OR it has no department but the role owns it. A
			// NULL-department lead is NOT leaked across departments — only branch-wide
			// roles (DepartmentID==nil) and the owner see it.
			sameDept := lead.DepartmentID != nil && *lead.DepartmentID == *scope.DepartmentID
			ownsNullDept := lead.DepartmentID == nil && scope.UserID != 0 && lead.OwnerID == scope.UserID
			if !sameDept && !ownsNullDept {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// clientMatchesScope returns true when client is accessible under the given scope.
func clientMatchesScope(scope DataScope, client *models.Client) bool {
	if client == nil {
		return false
	}
	switch scope.Kind {
	case ScopeKindAll:
		return true
	case ScopeKindOwn:
		return client.OwnerID == scope.UserID
	case ScopeKindBranch:
		// fail-closed: a branch-scoped check with no resolved branch must deny,
		// never allow-all (defense-in-depth against a half-built scope).
		if scope.BranchID == nil {
			return false
		}
		return client.BranchID != nil && *client.BranchID == *scope.BranchID
	default:
		return false
	}
}

// ─── Repo interface for scope-based deal listing ──────────────────────────────

// dealListRepo is the minimal interface covering listing methods of
// DealRepository needed for scope-based access control.
// *repositories.DealRepository satisfies this interface (duck typing).
type dealListRepo interface {
	ListAllWithFilterAndArchiveScope(limit, offset int, filter repositories.DealListFilter, scope repositories.ArchiveScope) ([]*models.Deals, error)
	CountAllWithFilterAndArchiveScope(filter repositories.DealListFilter, scope repositories.ArchiveScope) (int, error)
}

// ─── Deal scope ───────────────────────────────────────────────────────────────

// resolveDealScope returns the DataScope for the deals entity.
//
// DEALS mapping (preserves legacy branchScopeForRole semantics from deal_service.go):
//
//	admin / management                → All
//	sales / visa / quality_control    → Branch(user.BranchID)
//	partner / hr / legal / unknown    → Forbidden
func resolveDealScope(userID, roleID int, userRepo repositories.UserRepository) (DataScope, error) {
	switch roleID {
	case authz.RoleManagement, authz.RoleSystemAdmin, authz.RoleControl:
		// quality_control observes all deals (read-only enforced elsewhere).
		return DataScope{Kind: ScopeKindAll}, nil
	case authz.RoleSales, authz.RoleVisa:
		branchID, deptID, err := resolveUserContext(userID, userRepo)
		if err != nil {
			return DataScope{Kind: ScopeKindForbidden}, err
		}
		return DataScope{Kind: ScopeKindBranch, BranchID: branchID, DepartmentID: deptID}, nil
	default:
		return DataScope{Kind: ScopeKindForbidden}, ErrForbidden
	}
}

// dealMatchesScope returns true when deal is accessible under the given scope.
// Replaces the removed sameDealBranch helper from deal_service.go.
func dealMatchesScope(scope DataScope, deal *models.Deals) bool {
	if deal == nil {
		return false
	}
	switch scope.Kind {
	case ScopeKindAll:
		return true
	case ScopeKindBranch:
		if scope.BranchID != nil {
			if deal.BranchID == nil || *deal.BranchID != *scope.BranchID {
				return false
			}
		}
		if scope.DepartmentID != nil {
			// NULL fallback: deals with no department are visible within the branch.
			if deal.DepartmentID != nil && *deal.DepartmentID != *scope.DepartmentID {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// listDealsForScope executes the appropriate repository call based on the
// resolved DataScope. BranchID is injected into the filter for Branch scope.
func listDealsForScope(repo dealListRepo, scope DataScope, limit, offset int, filter repositories.DealListFilter, archiveScope repositories.ArchiveScope) ([]*models.Deals, error) {
	if scope.Kind == ScopeKindBranch {
		filter.BranchID = scope.BranchID
		filter.DepartmentID = scope.DepartmentID
	}
	return repo.ListAllWithFilterAndArchiveScope(limit, offset, filter, archiveScope)
}

// countDealsForScope executes the appropriate count query based on the
// resolved DataScope.
func countDealsForScope(repo dealListRepo, scope DataScope, filter repositories.DealListFilter, archiveScope repositories.ArchiveScope) (int, error) {
	if scope.Kind == ScopeKindBranch {
		filter.BranchID = scope.BranchID
		filter.DepartmentID = scope.DepartmentID
	}
	return repo.CountAllWithFilterAndArchiveScope(filter, archiveScope)
}
