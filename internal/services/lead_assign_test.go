package services

import (
	"errors"
	"strings"
	"testing"

	"turcompany/internal/authz"
	"turcompany/internal/models"
)

// leadOwnerUserRepo — сотрудники по id и их состояние учётки.
type leadOwnerUserRepo struct {
	docScopeUserRepoStub
	users  map[int]*models.User
	status map[int][2]bool // {active, verified}; нет записи — работает
}

func (r *leadOwnerUserRepo) GetByID(id int) (*models.User, error) { return r.users[id], nil }
func (r *leadOwnerUserRepo) GetAccountStatus(id int) (bool, bool, error) {
	if s, ok := r.status[id]; ok {
		return s[0], s[1], nil
	}
	return true, true, nil
}

const (
	tAdmin    = 1
	tManager  = 10
	tPeer     = 11
	tBlocked  = 12
	tPending  = 13
	tOtherBr  = 14
	tControl  = 15
	tMgmtElse = 16
)

func newLeadOwnerRepo() *leadOwnerUserRepo {
	return &leadOwnerUserRepo{
		users: map[int]*models.User{
			tAdmin:    {ID: tAdmin, RoleID: authz.RoleSystemAdmin},
			tManager:  {ID: tManager, RoleID: authz.RoleSales, BranchID: intPtr(1), LastName: "Иванов", FirstName: "Иван"},
			tPeer:     {ID: tPeer, RoleID: authz.RoleSales, BranchID: intPtr(1)},
			tBlocked:  {ID: tBlocked, RoleID: authz.RoleSales, BranchID: intPtr(1)},
			tPending:  {ID: tPending, RoleID: authz.RoleSales, BranchID: intPtr(1)},
			tOtherBr:  {ID: tOtherBr, RoleID: authz.RoleSales, BranchID: intPtr(2)},
			tControl:  {ID: tControl, RoleID: authz.RoleControl},
			tMgmtElse: {ID: tMgmtElse, RoleID: authz.RoleManagement, BranchID: intPtr(2)},
		},
		status: map[int][2]bool{
			tBlocked: {false, true},
			tPending: {true, false},
		},
	}
}

// Лид ничей, если ответственного нет, он уволен/не подтверждён или лид
// припаркован на админе — тогда его берёт себе любой сотрудник филиала.
func TestOwnerIsFree(t *testing.T) {
	svc := &LeadService{UserRepo: newLeadOwnerRepo()}
	cases := map[int]bool{0: true, tBlocked: true, tPending: true, tAdmin: true, tPeer: false}
	for owner, want := range cases {
		if got := svc.ownerIsFree(owner); got != want {
			t.Errorf("ownerIsFree(%d) = %v, want %v", owner, got, want)
		}
	}
	// Менеджер, перетащивший карточку уволенного коллеги, забирает её себе.
	if !svc.claimsOwnershipOnMove(&models.Leads{OwnerID: tBlocked}, tManager, authz.RoleSales) {
		t.Fatal("lead of a blocked employee must go to the manager who moves it")
	}
	if svc.claimsOwnershipOnMove(&models.Leads{OwnerID: tPeer}, tManager, authz.RoleSales) {
		t.Fatal("working colleague's lead must not be taken by moving the card")
	}
}

func TestCheckAssignee(t *testing.T) {
	svc := &LeadService{UserRepo: newLeadOwnerRepo()}
	lead := &models.Leads{ID: 5, BranchID: intPtr(1)}
	bad := map[int]string{
		tBlocked: "заблокирован",
		tPending: "не подтверждён",
		tControl: "не работает с лидами",
		tOtherBr: "другого филиала",
		999:      "не найден",
	}
	for id, want := range bad {
		_, err := svc.checkAssignee(lead, id)
		var ae *LeadAssignError
		if !errors.As(err, &ae) || !strings.Contains(ae.Msg, want) {
			t.Errorf("assignee %d: want error with %q, got %v", id, want, err)
		}
	}
	for _, id := range []int{tPeer, tMgmtElse, tAdmin} {
		if _, err := svc.checkAssignee(lead, id); err != nil {
			t.Errorf("assignee %d must be allowed, got %v", id, err)
		}
	}
	// Лид без филиала (общий пул) можно передать менеджеру любого филиала.
	if _, err := svc.checkAssignee(&models.Leads{ID: 6}, tOtherBr); err != nil {
		t.Errorf("lead without branch must be assignable to any branch, got %v", err)
	}
}

func TestDecorateOwner(t *testing.T) {
	svc := &LeadService{UserRepo: newLeadOwnerRepo()}
	lead := &models.Leads{OwnerID: tManager}
	svc.decorateOwner(lead)
	if lead.OwnerName != "Иванов Иван" || lead.Unassigned == nil || *lead.Unassigned {
		t.Fatalf("unexpected owner decoration: name=%q unassigned=%v", lead.OwnerName, lead.Unassigned)
	}
	free := &models.Leads{}
	svc.decorateOwner(free)
	if free.Unassigned == nil || !*free.Unassigned {
		t.Fatal("lead without owner must be marked unassigned")
	}
}

type fakeLeadReleaser struct{ released, releasedDeals []int }

func (f *fakeLeadReleaser) ReleaseLeadsOf(userID int) (int, error) {
	f.released = append(f.released, userID)
	return 1, nil
}

func (f *fakeLeadReleaser) ReleaseDealsOf(userID int) (int, error) {
	f.releasedDeals = append(f.releasedDeals, userID)
	return 1, nil
}

// Блокировка, снятие подтверждения и удаление освобождают лиды сотрудника.
func TestUserServiceReleasesLeadsOnDeactivation(t *testing.T) {
	rel := &fakeLeadReleaser{}
	svc := &userService{repo: &captureUserRepo{}, leads: rel, deals: rel}

	if err := svc.UpdateUser(&models.User{ID: 7, IsActive: true, IsVerified: true}); err != nil {
		t.Fatal(err)
	}
	if len(rel.released) != 0 || len(rel.releasedDeals) != 0 {
		t.Fatalf("working employee keeps leads and deals, released %v / %v", rel.released, rel.releasedDeals)
	}
	_ = svc.UpdateUser(&models.User{ID: 7, IsActive: false, IsVerified: true})
	_ = svc.UpdateUser(&models.User{ID: 8, IsActive: true, IsVerified: false})
	_ = svc.DeleteUser(9)
	if len(rel.released) != 3 || rel.released[0] != 7 || rel.released[1] != 8 || rel.released[2] != 9 {
		t.Fatalf("blocked/unverified/deleted must release leads, got %v", rel.released)
	}
	if len(rel.releasedDeals) != 3 || rel.releasedDeals[2] != 9 {
		t.Fatalf("blocked/unverified/deleted must release deals, got %v", rel.releasedDeals)
	}
}
