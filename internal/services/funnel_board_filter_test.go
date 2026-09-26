package services

import (
	"testing"

	"turcompany/internal/authz"
)

// Обратная связь заказчика 17.09.2026: «менеджеры видят все общие лиды которые
// поступают и только свои по умолчанию, друг друга не должны видеть, но должен
// быть пункт отобразить все лиды филиала».
func TestResolveBoardFilter_ManagerDefaultsToOwnPlusUnowned(t *testing.T) {
	for _, roleID := range []int{authz.RoleSales, authz.RoleVisa, authz.RolePartner} {
		f := resolveBoardFilter(BoardQuery{}, 42, roleID)
		if f.OwnerID == nil || *f.OwnerID != 42 {
			t.Fatalf("role %d: expected own cards by default, got %+v", roleID, f.OwnerID)
		}
		if !f.IncludeUnowned {
			t.Fatalf("role %d: new/unassigned leads must stay visible by default", roleID)
		}
	}
}

func TestResolveBoardFilter_SupervisorsSeeEverythingByDefault(t *testing.T) {
	for _, roleID := range []int{authz.RoleManagement, authz.RoleSystemAdmin, authz.RoleControl} {
		f := resolveBoardFilter(BoardQuery{}, 42, roleID)
		if f.OwnerID != nil {
			t.Fatalf("role %d: supervisors must not be narrowed to own cards, got %+v", roleID, f.OwnerID)
		}
	}
}

// «Все лиды филиала» — ролевой scope филиала при этом никуда не девается,
// он применяется отдельно в Board().
func TestResolveBoardFilter_AllScopeDropsOwnerCondition(t *testing.T) {
	f := resolveBoardFilter(BoardQuery{OwnerScope: BoardOwnerScopeAll}, 42, authz.RoleSales)
	if f.OwnerID != nil {
		t.Fatalf("owner=all must not filter by owner, got %+v", f.OwnerID)
	}
	if f.IncludeUnowned {
		t.Fatal("owner=all needs no unowned widening — everything is already included")
	}
}

// Явный выбор менеджера («сортировка по менеджерам») показывает только его
// карточки, без общего пула — иначе фильтр бесполезен.
func TestResolveBoardFilter_ExplicitOwnerWins(t *testing.T) {
	other := 7
	f := resolveBoardFilter(BoardQuery{OwnerID: &other, OwnerScope: BoardOwnerScopeMine}, 42, authz.RoleSales)
	if f.OwnerID == nil || *f.OwnerID != 7 {
		t.Fatalf("explicit owner_id must win, got %+v", f.OwnerID)
	}
	if f.IncludeUnowned {
		t.Fatal("explicit manager filter must not mix in unowned cards")
	}
}

func TestResolveBoardFilter_KeepsSearchQuery(t *testing.T) {
	f := resolveBoardFilter(BoardQuery{Query: "  Асқар "}, 42, authz.RoleSales)
	if f.Query != "  Асқар " {
		t.Fatalf("search query must reach the repository as-is, got %q", f.Query)
	}
}

// Входящий лид (Instagram/WhatsApp/звонок) и лид от админа висят на аккаунте
// админа/руководства — для менеджера такая карточка «ничья». Список ролей
// обязан совпадать с трактовкой переноса карточки (claimsOwnershipOnMove),
// иначе «новые заявки» перестанут показываться в режиме по умолчанию.
func TestResolveBoardFilter_UnownedRolesMatchOwnershipClaimRule(t *testing.T) {
	f := resolveBoardFilter(BoardQuery{}, 42, authz.RoleSales)
	if len(f.UnownedRoleIDs) == 0 {
		t.Fatal("default manager view must treat admin-parked cards as unclaimed")
	}
	for _, roleID := range f.UnownedRoleIDs {
		if !authz.IsElevated(roleID) {
			t.Fatalf("role %d is listed as an unclaimed-parking role but is not elevated", roleID)
		}
	}
	for _, roleID := range []int{authz.RoleSystemAdmin, authz.RoleManagement, authz.RoleControl} {
		found := false
		for _, got := range f.UnownedRoleIDs {
			if got == roleID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("elevated role %d must be treated as an unclaimed-parking role", roleID)
		}
	}
}

// Фильтр админа «весь филиал» доходит до репозитория в любом режиме — и при
// «все лиды», и вместе с выбранным сотрудником.
func TestResolveBoardFilter_BranchPassesThrough(t *testing.T) {
	branch, other := 3, 7
	f := resolveBoardFilter(BoardQuery{OwnerScope: BoardOwnerScopeAll, BranchID: &branch}, 42, authz.RoleSystemAdmin)
	if f.BranchID == nil || *f.BranchID != 3 || f.OwnerID != nil {
		t.Fatalf("branch filter must narrow to branch only, got %+v", f)
	}
	f = resolveBoardFilter(BoardQuery{OwnerID: &other, BranchID: &branch}, 42, authz.RoleSystemAdmin)
	if f.BranchID == nil || *f.BranchID != 3 || f.OwnerID == nil || *f.OwnerID != 7 {
		t.Fatalf("branch must survive explicit owner, got %+v", f)
	}
}
