package wazzup

import (
	"context"
	"errors"
	"testing"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

// rolesRepo — два аккаунта; у дочернего номер «Алматы» с ручным доступом и
// номер «Шымкент» с автоматическим.
type rolesRepo struct {
	multiRepo
	users      []repositories.CRMUserDTO
	candidates []repositories.RoleCandidateDTO
	manual     map[int64][]repositories.ChannelUserRole
	replaced   []repositories.ChannelUserRole
}

func (r *rolesRepo) ListCRMUsers(context.Context) ([]repositories.CRMUserDTO, error) {
	return r.users, nil
}
func (r *rolesRepo) ListRoleCandidates(context.Context) ([]repositories.RoleCandidateDTO, error) {
	return r.candidates, nil
}
func (r *rolesRepo) ListChannelRoles(_ context.Context, channelID int64) ([]repositories.ChannelUserRole, error) {
	return r.manual[channelID], nil
}
func (r *rolesRepo) ReplaceChannelRoles(_ context.Context, channelID int64, roles []repositories.ChannelUserRole) error {
	r.replaced = roles
	r.manual[channelID] = roles
	for id, list := range r.channels {
		for i := range list {
			if list[i].ID == channelID {
				r.channels[id][i].RolesConfigured = true
			}
		}
	}
	return nil
}

// rolesClient запоминает роли, отправленные в Wazzup.
type rolesClient struct {
	noopClient
	sent *[]UserChannelRole
}

func (c rolesClient) SyncUserRoles(_ context.Context, _ string, roles []UserChannelRole) error {
	*c.sent = roles
	return nil
}

const (
	userSales   = 1 // менеджер Алматы
	userSales2  = 2 // менеджер Шымкента
	userManager = 3 // руководство
	userFired   = 4 // уволен, но остался в настройке номера
)

func newRolesService(t *testing.T) (*Service, *rolesRepo, *[]UserChannelRole) {
	t.Helper()
	sent := &[]UserChannelRole{}
	repo := &rolesRepo{
		multiRepo: multiRepo{
			integrations: map[string]*models.WazzupIntegration{
				AccountMain:  {ID: 1, Enabled: true, Account: AccountMain},
				AccountChild: {ID: 2, Enabled: true, Account: AccountChild},
			},
			channels: map[int][]models.WazzupChannel{
				1: {{ID: 11, IntegrationID: 1, ExternalChannelID: "main-almaty"}},
				2: {
					{ID: 21, IntegrationID: 2, ExternalChannelID: "child-almaty", RolesConfigured: true},
					{ID: 22, IntegrationID: 2, ExternalChannelID: "child-shymkent"},
				},
			},
		},
		users: []repositories.CRMUserDTO{
			{ID: userSales, RoleID: authz.RoleSales},
			{ID: userSales2, RoleID: authz.RoleSales},
			{ID: userManager, RoleID: authz.RoleManagement},
		},
		candidates: []repositories.RoleCandidateDTO{
			{ID: userSales, Name: "Алсейтова", RoleID: authz.RoleSales, BranchName: "Алматы"},
			{ID: userSales2, Name: "Кулжанова", RoleID: authz.RoleSales, BranchName: "Шымкент"},
			{ID: userManager, Name: "Камиля", RoleID: authz.RoleManagement},
		},
		manual: map[int64][]repositories.ChannelUserRole{
			21: {
				{UserID: userSales, Role: "seller", AllowGetNewClients: true},
				{UserID: userManager, Role: "manager"},
				{UserID: userFired, Role: "seller", AllowGetNewClients: true},
			},
		},
	}
	svc := NewService(repo, recClient{name: "main", calls: &[]string{}}, "main-key", "", "", "")
	svc.RegisterAccount(AccountConfig{Name: AccountChild, Client: rolesClient{sent: sent}, Partner: true})
	return svc, repo, sent
}

// wz — id сотрудника CRM в Wazzup.
func wz(userID int) string { return wazzupUserIDFor(userID, userID) }

func rolesByChannel(sent []UserChannelRole) map[string]map[string]UserChannelRole {
	out := map[string]map[string]UserChannelRole{}
	for _, r := range sent {
		if out[r.ChannelID] == nil {
			out[r.ChannelID] = map[string]UserChannelRole{}
		}
		out[r.ChannelID][r.UserID] = r
	}
	return out
}

// Номер с ручным доступом получает ровно сохранённые роли, остальные номера —
// автоматические, как раньше. Уволенный сотрудник доступа не получает.
func TestSyncUserRolesUsesManualAccessPerChannel(t *testing.T) {
	svc, _, sent := newRolesService(t)

	if _, err := svc.SyncChannels(context.Background(), 5); err != nil {
		t.Fatal(err)
	}
	got := rolesByChannel(*sent)

	almaty := got["child-almaty"]
	if len(almaty) != 2 {
		t.Fatalf("manual channel must get only saved active users, got %+v", almaty)
	}
	if r := almaty[wz(1)]; r.Role != "seller" || !r.AllowGetNewClients {
		t.Fatalf("saved seller role lost: %+v", r)
	}
	if r := almaty[wz(3)]; r.Role != "manager" || r.AllowGetNewClients {
		t.Fatalf("saved manager role lost: %+v", r)
	}
	if _, ok := almaty[wz(2)]; ok {
		t.Fatal("employee without a saved role must not get access to the manual channel")
	}
	if _, ok := almaty[wz(4)]; ok {
		t.Fatal("fired employee must not get access")
	}

	if shymkent := got["child-shymkent"]; len(shymkent) != 3 || shymkent[wz(2)].Role != "seller" || shymkent[wz(3)].Role != "manager" {
		t.Fatalf("automatic channel must keep role-based access, got %+v", shymkent)
	}
}

func TestSetChannelRolesSavesAndPushes(t *testing.T) {
	svc, repo, sent := newRolesService(t)

	view, err := svc.SetChannelRoles(context.Background(), 22, []ChannelRoleInput{
		{UserID: userSales2, Role: "seller", AllowGetNewClients: true},
		{UserID: userManager, Role: "auditor"},
		{UserID: userSales, Role: ""}, // без доступа
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.replaced) != 2 {
		t.Fatalf("only employees with a role must be saved, got %+v", repo.replaced)
	}
	shymkent := rolesByChannel(*sent)["child-shymkent"]
	if len(shymkent) != 2 || shymkent[wz(2)].Role != "seller" || shymkent[wz(3)].Role != "auditor" {
		t.Fatalf("saved access must be pushed to Wazzup at once, got %+v", shymkent)
	}
	if !view.Configured {
		t.Fatal("view must report manual access after saving")
	}
}

func TestSetChannelRolesRejectsBadInput(t *testing.T) {
	svc, _, _ := newRolesService(t)

	if _, err := svc.SetChannelRoles(context.Background(), 22, []ChannelRoleInput{{UserID: userSales, Role: "owner"}}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("unknown role must be rejected, got %v", err)
	}
	if _, err := svc.SetChannelRoles(context.Background(), 22, []ChannelRoleInput{{UserID: userFired, Role: "seller"}}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("inactive employee must be rejected, got %v", err)
	}
}

// У основного аккаунта свой кабинет — CRM его роли не трогает.
func TestChannelRolesRejectsCabinetAccount(t *testing.T) {
	svc, _, _ := newRolesService(t)

	if _, err := svc.ChannelRoles(context.Background(), 11); !errors.Is(err, ErrRolesManagedInCabinet) {
		t.Fatalf("main account channel must be managed in the cabinet, got %v", err)
	}
}

// Пока доступ не настроен вручную, окно показывает автоматические роли.
func TestChannelRolesShowsAutomaticAccess(t *testing.T) {
	svc, _, _ := newRolesService(t)

	view, err := svc.ChannelRoles(context.Background(), 22)
	if err != nil {
		t.Fatal(err)
	}
	if view.Configured || len(view.Items) != 3 {
		t.Fatalf("unexpected view: %+v", view)
	}
	for _, it := range view.Items {
		want := wazzupRoleFor(it.CRMRoleID)
		if it.Role != want || it.AllowGetNewClients != (want == "seller") {
			t.Fatalf("automatic access mismatch for %d: %+v", it.UserID, it)
		}
	}
}
