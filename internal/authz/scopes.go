package authz

const (
	ScopeOwn                = "own"
	ScopeAssigned           = "assigned"
	ScopeDepartment         = "department"
	ScopeBranch             = "branch"
	ScopeRelatedDepartments = "related_departments"
	ScopeAll                = "all"
)

func ScopeRank(scope string) int {
	switch scope {
	case ScopeOwn:
		return 10
	case ScopeAssigned:
		return 20
	case ScopeDepartment:
		return 30
	case ScopeBranch:
		return 40
	case ScopeRelatedDepartments:
		return 50
	case ScopeAll:
		return 60
	default:
		return 0
	}
}
