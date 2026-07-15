package services

import (
	"testing"

	"turcompany/internal/authz"
	"turcompany/internal/models"
)

// Кто забирает лид себе, перетащив карточку (обратная связь 14.07.2026).
// Входящие лиды из Instagram/WhatsApp приезжают на аккаунт админа интеграции,
// поэтому менеджер должен их подхватывать; чужой рабочий лид — не трогать.
func TestClaimsOwnershipOnMove(t *testing.T) {
	const (
		managerID = 10
		peerID    = 11
		adminID   = 1
	)

	ownerRole := func(roleID int) *docScopeUserRepoStub {
		return &docScopeUserRepoStub{user: &models.User{RoleID: roleID}}
	}

	tests := []struct {
		name      string
		lead      *models.Leads
		actorID   int
		actorRole int
		ownerRepo *docScopeUserRepoStub
		want      bool
	}{
		{
			name:      "менеджер тащит лид с админа — забирает себе",
			lead:      &models.Leads{OwnerID: adminID},
			actorID:   managerID,
			actorRole: authz.RoleSales,
			ownerRepo: ownerRole(authz.RoleSystemAdmin),
			want:      true,
		},
		{
			name:      "менеджер тащит лид с руководства — забирает себе",
			lead:      &models.Leads{OwnerID: adminID},
			actorID:   managerID,
			actorRole: authz.RoleSales,
			ownerRepo: ownerRole(authz.RoleManagement),
			want:      true,
		},
		{
			name:      "менеджер тащит ничейный лид — забирает себе",
			lead:      &models.Leads{OwnerID: 0},
			actorID:   managerID,
			actorRole: authz.RoleSales,
			ownerRepo: ownerRole(authz.RoleSystemAdmin),
			want:      true,
		},
		{
			name:      "менеджер тащит лид коллеги — владелец не меняется",
			lead:      &models.Leads{OwnerID: peerID},
			actorID:   managerID,
			actorRole: authz.RoleSales,
			ownerRepo: ownerRole(authz.RoleSales),
			want:      false,
		},
		{
			name:      "менеджер тащит свой лид — менять нечего",
			lead:      &models.Leads{OwnerID: managerID},
			actorID:   managerID,
			actorRole: authz.RoleSales,
			ownerRepo: ownerRole(authz.RoleSales),
			want:      false,
		},
		{
			name:      "админ тащит лид менеджера — себе не забирает",
			lead:      &models.Leads{OwnerID: peerID},
			actorID:   adminID,
			actorRole: authz.RoleSystemAdmin,
			ownerRepo: ownerRole(authz.RoleSales),
			want:      false,
		},
		{
			name:      "визовый отдел тащит лид с админа — забирает себе",
			lead:      &models.Leads{OwnerID: adminID},
			actorID:   managerID,
			actorRole: authz.RoleVisa,
			ownerRepo: ownerRole(authz.RoleSystemAdmin),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &LeadService{UserRepo: tt.ownerRepo}
			got := svc.claimsOwnershipOnMove(tt.lead, tt.actorID, tt.actorRole)
			if got != tt.want {
				t.Fatalf("claimsOwnershipOnMove = %v, ожидалось %v", got, tt.want)
			}
		})
	}
}

// Без UserRepo роль владельца не установить — не забираем лид молча.
func TestClaimsOwnershipOnMoveWithoutUserRepo(t *testing.T) {
	svc := &LeadService{}
	if svc.claimsOwnershipOnMove(&models.Leads{OwnerID: 1}, 10, authz.RoleSales) {
		t.Fatal("без UserRepo лид не должен переходить к менеджеру")
	}
}
