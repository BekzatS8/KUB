package services

import (
	"testing"

	"turcompany/internal/authz"
	"turcompany/internal/models"
)

// Block C: quality_control observes ALL clients (read-only enforced elsewhere),
// not just its own branch.
func TestClientScope_ControlSeesAll(t *testing.T) {
	branchID := 8
	userRepo := &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}

	scope, err := resolveClientScope(100, authz.RoleControl, userRepo)
	if err != nil {
		t.Fatalf("resolveClientScope failed: %v", err)
	}
	if scope.Kind != ScopeKindAll {
		t.Fatalf("control must observe all clients (ScopeKindAll), got %+v", scope)
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

// Обратная связь заказчика 17.09.2026: все менеджеры видят информацию всех
// клиентов. Чтение (карточка + списки) идёт через resolveClientReadScope и
// обязано быть ScopeKindAll для МОП/визового/партнёрского отделов, иначе
// «глазок» в списке снова даёт «У вас нет доступа к этому клиенту».
func TestClientReadScope_AllManagerRolesSeeAllClients(t *testing.T) {
	branchID := 8
	userRepo := &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}

	roles := []int{
		authz.RoleSales, authz.RoleVisa, authz.RolePartner,
		authz.RoleManagement, authz.RoleSystemAdmin, authz.RoleControl, authz.RoleLegal,
	}
	for _, roleID := range roles {
		scope, err := resolveClientReadScope(100, roleID, userRepo)
		if err != nil {
			t.Fatalf("role %d: resolveClientReadScope failed: %v", roleID, err)
		}
		if scope.Kind != ScopeKindAll {
			t.Fatalf("role %d: expected ScopeKindAll for client reads, got %+v", roleID, scope)
		}
	}
}

// Запись по-прежнему филиальная: МОП не должен редактировать чужих клиентов.
func TestClientWriteScope_SalesStaysBranchScoped(t *testing.T) {
	branchID := 8
	userRepo := &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}

	scope, err := resolveClientScope(100, authz.RoleSales, userRepo)
	if err != nil {
		t.Fatalf("resolveClientScope failed: %v", err)
	}
	if scope.Kind != ScopeKindBranch {
		t.Fatalf("sales write scope must stay ScopeKindBranch, got %+v", scope)
	}
}
