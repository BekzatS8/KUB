package wazzup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"turcompany/internal/repositories"
)

// contactsClient — клиент, запоминающий назначения (и ключ, с которым они
// пришли) и пользователей.
type contactsClient struct {
	rolesClient
	assigned *[]ContactUpsert
	keys     *[]string
	users    *[]UserUpsert
	deals    *[]DealUpsert
}

func (c contactsClient) UpsertDeals(_ context.Context, _ string, deals []DealUpsert) error {
	if c.deals != nil {
		*c.deals = append(*c.deals, deals...)
	}
	return nil
}

func (c contactsClient) UpsertContacts(_ context.Context, apiKey string, contacts []ContactUpsert) error {
	*c.assigned = append(*c.assigned, contacts...)
	if c.keys != nil {
		*c.keys = append(*c.keys, apiKey)
	}
	return nil
}

func (c contactsClient) UpsertUsers(_ context.Context, _ string, users []UserUpsert) error {
	if c.users != nil {
		*c.users = append(*c.users, users...)
	}
	return nil
}

// Менеджер (seller) пишет первым — клиент переходит к нему: иначе Wazzup
// ответит chat_no_access, если клиент закреплён за другим сотрудником.
func TestSellerWritingFirstBecomesResponsible(t *testing.T) {
	svc, _, sent := newRolesService(t)
	assigned := &[]ContactUpsert{}
	svc.accounts[1].Client = contactsClient{rolesClient: rolesClient{sent: sent}, assigned: assigned}
	svc.SetCRMBaseURL("https://kubcrm.kz/")

	if _, err := svc.SendMessage(context.Background(), userSales, "77001112233", "whatsapp", "child-almaty", "привет"); err != nil {
		t.Fatal(err)
	}
	if len(*assigned) != 1 {
		t.Fatalf("seller writing first must become responsible, got %+v", *assigned)
	}
	got := (*assigned)[0]
	if got.ResponsibleUserID != wz(userSales) || got.ChatID != "77001112233" || got.ChatType != "whatsapp" ||
		got.ID != "kub-whatsapp-77001112233" || got.URI != "https://kubcrm.kz/leads/by-chat?transport=whatsapp&chat_id=77001112233" {
		t.Fatalf("unexpected contact: %+v", got)
	}

	// Руководитель видит все чаты — клиентов у менеджеров не забираем.
	if _, err := svc.SendMessage(context.Background(), userManager, "77009998877", "whatsapp", "child-almaty", "привет"); err != nil {
		t.Fatal(err)
	}
	if len(*assigned) != 1 {
		t.Fatalf("head must not take over clients, got %+v", *assigned)
	}
}

func TestPartnerClientUpsertContactsPayload(t *testing.T) {
	var gotPath string
	var got map[string]any
	client, _, closeFn := newTestPartnerClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"processed":1},"meta":{}}`)
	})
	defer closeFn()

	err := client.UpsertContacts(context.Background(), "", []ContactUpsert{{
		ID: "kub-whatsapp-77001112233", ResponsibleUserID: "kub-34-34", Name: "77001112233",
		ChatType: "whatsapp", ChatID: "77001112233", URI: "https://kubcrm.kz/whatsapp",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "POST /v2/contacts" {
		t.Fatalf("unexpected request %s", gotPath)
	}
	contacts, _ := got["contacts"].([]any)
	if len(contacts) != 1 {
		t.Fatalf("contacts payload mismatch: %v", got)
	}
	c := contacts[0].(map[string]any)
	data, _ := c["contact_data"].([]any)
	if c["responsible_user_id"] != "kub-34-34" || c["id"] != "kub-whatsapp-77001112233" || len(data) != 1 ||
		data[0].(map[string]any)["chat_id"] != "77001112233" || data[0].(map[string]any)["chat_type"] != "whatsapp" {
		t.Fatalf("contact payload mismatch: %v", c)
	}
}

// chatOwnerRepo — переписка принадлежит лиду с ответственным owner.
type chatOwnerRepo struct {
	*rolesRepo
	owner int
}

func (r chatOwnerRepo) ChatLeadOwner(context.Context, string, string) (int, error) { return r.owner, nil }

// Взяли или передали лид — переписка в аккаунте без кабинета переходит новому
// ответственному: иначе «Открыть в мессенджере» показывало чужой чат.
func TestAssignChatOwnerFollowsLead(t *testing.T) {
	svc, repo, sent := newRolesService(t)
	assigned := &[]ContactUpsert{}
	svc.accounts[1].Client = contactsClient{rolesClient: rolesClient{sent: sent}, assigned: assigned}
	repo.chatChannel = map[string]string{"77001112233": "child-almaty", "77005556677": "main-almaty"}

	svc.AssignChatOwner(context.Background(), "whatsapp", "+7 700 111 22 33", userSales2)
	if len(*assigned) != 1 || (*assigned)[0].ResponsibleUserID != wz(userSales2) || (*assigned)[0].ChatID != "77001112233" {
		t.Fatalf("chat must follow the lead to the new owner, got %+v", *assigned)
	}
	// Основной аккаунт тоже (замечание 19.11.2026): ответственный за лид
	// становится ответственным за чат, а чат получает ссылку на лид. Ключ —
	// ключ основного аккаунта.
	mainAssigned, mainKeys := &[]ContactUpsert{}, &[]string{}
	svc.accounts[0].Client = contactsClient{rolesClient: rolesClient{sent: sent}, assigned: mainAssigned, keys: mainKeys}
	svc.AssignChatOwner(context.Background(), "whatsapp", "77005556677", userSales2)
	if len(*mainAssigned) != 1 || (*mainAssigned)[0].ResponsibleUserID != wz(userSales2) || (*mainKeys)[0] != "main-key" {
		t.Fatalf("main account chat must follow the lead too, got %+v keys %v", *mainAssigned, *mainKeys)
	}
	// Незнакомые переписки не трогаем.
	svc.AssignChatOwner(context.Background(), "whatsapp", "77000000000", userSales2)
	if len(*assigned) != 1 || len(*mainAssigned) != 1 {
		t.Fatalf("unknown chats must stay untouched, got %+v / %+v", *assigned, *mainAssigned)
	}

	// Открывает переписку ответственный за её лид — чат закрепляется за ним;
	// чужой сотрудник ничего не забирает.
	svc.repo = chatOwnerRepo{rolesRepo: repo, owner: userSales}
	svc.assignOwnChatOnOpen(context.Background(), svc.accounts[1], "whatsapp", "77001112233", userSales2)
	if len(*assigned) != 1 {
		t.Fatalf("someone else opening the chat must not take it, got %+v", *assigned)
	}
	svc.assignOwnChatOnOpen(context.Background(), svc.accounts[1], "whatsapp", "77001112233", userSales)
	if len(*assigned) != 2 || (*assigned)[1].ResponsibleUserID != wz(userSales) {
		t.Fatalf("lead owner opening the chat must get it, got %+v", *assigned)
	}
}

func TestHTTPClientUpsertContactsPayload(t *testing.T) {
	var gotPath, gotAuth string
	var got []map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `"OK"`)
	}))
	defer ts.Close()

	c := NewHTTPClient(ts.URL, 2*time.Second, 0, 10*time.Millisecond)
	err := c.UpsertContacts(context.Background(), "main-key", []ContactUpsert{{
		ID: "kub-telegram-4982", ResponsibleUserID: "kub-34-34", Name: "Айгерим",
		ChatType: "tg", ChatID: "4982", URI: "https://kubcrm.kz/leads/by-chat?transport=telegram&chat_id=4982",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "POST /v3/contacts" || gotAuth != "Bearer main-key" || len(got) != 1 {
		t.Fatalf("unexpected request %s auth=%q body=%v", gotPath, gotAuth, got)
	}
	data, _ := got[0]["contactData"].([]any)
	if got[0]["responsibleUserId"] != "kub-34-34" || got[0]["name"] != "Айгерим" || got[0]["uri"] == "" || len(data) != 1 ||
		data[0].(map[string]any)["chatType"] != "telegram" || data[0].(map[string]any)["chatId"] != "4982" {
		t.Fatalf("contact payload mismatch: %v", got[0])
	}
}

// leadContactsRepo — переписки с ответственными за лиды.
type leadContactsRepo struct {
	*rolesRepo
	rows    []repositories.ChatLeadContact
	account string
}

func (r *leadContactsRepo) ListChatLeadContacts(_ context.Context, account string) ([]repositories.ChatLeadContact, error) {
	r.account = account
	return r.rows, nil
}

// При старте контакты всех переписок с ответственным выгружаются в Wazzup
// пачками по 100: ответственный = ответственный за лид, ссылка — на лид.
func TestSyncLeadContactsUploadsInBatches(t *testing.T) {
	svc, roles, sent := newRolesService(t)
	svc.accounts = svc.accounts[:1] // только основной аккаунт
	svc.SetCRMBaseURL("https://kubcrm.kz")
	rows := make([]repositories.ChatLeadContact, 0, 150)
	for i := 0; i < 150; i++ {
		owner := userSales
		if i%2 == 1 {
			owner = userSales2
		}
		rows = append(rows, repositories.ChatLeadContact{Transport: "whatsapp", ChatID: fmt.Sprintf("7700%07d", i), OwnerID: owner, OwnerName: "Менеджер", LeadID: 1000 + i, LeadTitle: "Лид из WhatsApp"})
	}
	rows[0].Name = "Айгерим"
	rows[1] = repositories.ChatLeadContact{Transport: "instagram", ChatID: "happy_client", OwnerID: userSales2, OwnerName: "Кулжанова", LeadID: 77, LeadClosed: true}
	repo := &leadContactsRepo{rolesRepo: roles, rows: rows}
	svc.repo = repo

	assigned, keys, users, deals := &[]ContactUpsert{}, &[]string{}, &[]UserUpsert{}, &[]DealUpsert{}
	svc.accounts[0].Client = contactsClient{rolesClient: rolesClient{sent: sent}, assigned: assigned, keys: keys, users: users, deals: deals}
	svc.SyncLeadContacts(context.Background())

	if repo.account != AccountMain {
		t.Fatalf("contacts must be listed for the main account, got %q", repo.account)
	}
	if len(*assigned) != 150 || len(*keys) != 2 || (*keys)[0] != "main-key" {
		t.Fatalf("expected 150 contacts in 2 batches, got %d in %d", len(*assigned), len(*keys))
	}
	if len(*users) != 2 {
		t.Fatalf("each owner must be a Wazzup user once, got %+v", *users)
	}
	first, insta := (*assigned)[0], (*assigned)[1]
	if first.Name != "Айгерим" || first.ResponsibleUserID != wz(userSales) || first.ID != "kub-whatsapp-77000000000" ||
		first.URI != "https://kubcrm.kz/leads/by-chat?transport=whatsapp&chat_id=77000000000" {
		t.Fatalf("unexpected contact: %+v", first)
	}
	if insta.Name != "happy_client" || insta.ChatType != "instagram" || insta.ResponsibleUserID != wz(userSales2) {
		t.Fatalf("unnamed contact must fall back to chat id: %+v", insta)
	}
	// Лид — сделка в шапке чата «Сделки» со ссылкой на карточку лида.
	if len(*deals) != 150 {
		t.Fatalf("each chat lead must become a deal, got %d", len(*deals))
	}
	d0, d1 := (*deals)[0], (*deals)[1]
	if d0.ID != "1000" || d0.URI != "https://kubcrm.kz/leads/1000" || d0.Name != "Лид #1000 — Лид из WhatsApp" ||
		d0.Closed || d0.ResponsibleUserID != wz(userSales) || len(d0.ContactIDs) != 1 || d0.ContactIDs[0] != first.ID {
		t.Fatalf("unexpected deal: %+v", d0)
	}
	if d1.Name != "Лид #77" || !d1.Closed || d1.ContactIDs[0] != "kub-instagram-happy_client" {
		t.Fatalf("closed lead must be a closed deal: %+v", d1)
	}
}

func TestHTTPClientUpsertDealsPayload(t *testing.T) {
	var gotPath string
	var got []map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		_, _ = io.WriteString(w, `"OK"`)
	}))
	defer ts.Close()

	c := NewHTTPClient(ts.URL, 2*time.Second, 0, 10*time.Millisecond)
	if err := c.UpsertDeals(context.Background(), "main-key", []DealUpsert{{
		ID: "77", ResponsibleUserID: "kub-34-34", Name: "Лид #77", URI: "https://kubcrm.kz/leads/77",
		ContactIDs: []string{"kub-whatsapp-77001112233"}, Closed: false,
	}}); err != nil {
		t.Fatal(err)
	}
	contacts, _ := got[0]["contacts"].([]any)
	if gotPath != "POST /v3/deals" || got[0]["id"] != "77" || got[0]["uri"] != "https://kubcrm.kz/leads/77" ||
		got[0]["closed"] != false || len(contacts) != 1 || got[0]["responsibleUserId"] != "kub-34-34" {
		t.Fatalf("unexpected deals request %s %v", gotPath, got)
	}
}

// chatLeadRepo — у переписки есть лид.
type chatLeadRepo struct {
	*rolesRepo
	lead *repositories.ChatLeadContact
}

func (r chatLeadRepo) ChatLead(context.Context, string, string) (*repositories.ChatLeadContact, error) {
	return r.lead, nil
}

// Взяли лид — в шапке чата появляется и сделка со ссылкой на лид.
func TestAssignChatOwnerPushesLeadDeal(t *testing.T) {
	svc, roles, sent := newRolesService(t)
	roles.chatChannel = map[string]string{"77005556677": "main-almaty"}
	svc.repo = chatLeadRepo{rolesRepo: roles, lead: &repositories.ChatLeadContact{Name: "Айгерим", LeadID: 42, LeadTitle: "Лид из WhatsApp"}}
	svc.SetCRMBaseURL("https://kubcrm.kz")
	assigned, deals := &[]ContactUpsert{}, &[]DealUpsert{}
	svc.accounts[0].Client = contactsClient{rolesClient: rolesClient{sent: sent}, assigned: assigned, deals: deals}

	svc.AssignChatOwner(context.Background(), "whatsapp", "77005556677", userSales)
	if len(*assigned) != 1 || (*assigned)[0].Name != "Айгерим" {
		t.Fatalf("contact must carry the client name, got %+v", *assigned)
	}
	if len(*deals) != 1 || (*deals)[0].URI != "https://kubcrm.kz/leads/42" || (*deals)[0].ResponsibleUserID != wz(userSales) ||
		(*deals)[0].ContactIDs[0] != (*assigned)[0].ID {
		t.Fatalf("lead deal must follow the contact, got %+v", *deals)
	}
}

// mergedLeadRepo — лид, в который влит лид из Instagram: две переписки.
type mergedLeadRepo struct {
	*leadContactsRepo
	chats  []repositories.ChatLeadContact
	merged []repositories.MergedLead
}

func (r mergedLeadRepo) LeadChats(context.Context, int, string) ([]repositories.ChatLeadContact, error) {
	return r.chats, nil
}

func (r mergedLeadRepo) ListMergedLeads(context.Context) ([]repositories.MergedLead, error) {
	return r.merged, nil
}

func (r mergedLeadRepo) ChatLead(context.Context, string, string) (*repositories.ChatLeadContact, error) {
	c := r.chats[0]
	return &c, nil
}

// Замечание 20.11.2026: лид из Instagram объединили с лидом из WhatsApp — из
// чата Instagram ссылка вела на удалённый лид. Сделка основного лида видна в
// обоих чатах, сделка влитого лида закрыта и ведёт на основной.
func TestSyncLeadContactsMergedLeadDeals(t *testing.T) {
	svc, roles, sent := newRolesService(t)
	svc.accounts = svc.accounts[:1]
	svc.SetCRMBaseURL("https://kubcrm.kz")
	rows := []repositories.ChatLeadContact{
		{Transport: "instagram", ChatID: "indira", OwnerID: userSales, LeadID: 539, LeadTitle: "Лид из WhatsApp"},
		{Transport: "whatsapp", ChatID: "77775120593", OwnerID: userSales, LeadID: 539, LeadTitle: "Лид из WhatsApp"},
	}
	svc.repo = mergedLeadRepo{
		leadContactsRepo: &leadContactsRepo{rolesRepo: roles, rows: rows},
		merged:           []repositories.MergedLead{{SourceID: 518, TargetID: 539}, {SourceID: 1, TargetID: 999}},
	}
	assigned, keys, users, deals := &[]ContactUpsert{}, &[]string{}, &[]UserUpsert{}, &[]DealUpsert{}
	svc.accounts[0].Client = contactsClient{rolesClient: rolesClient{sent: sent}, assigned: assigned, keys: keys, users: users, deals: deals}
	svc.SyncLeadContacts(context.Background())

	if len(*deals) != 2 {
		t.Fatalf("expected the lead deal and the merged deal, got %+v", *deals)
	}
	lead, merged := (*deals)[0], (*deals)[1]
	if lead.ID != "539" || len(lead.ContactIDs) != 2 || lead.ContactIDs[0] != "kub-instagram-indira" || lead.ContactIDs[1] != "kub-whatsapp-77775120593" {
		t.Fatalf("lead deal must reference both chats: %+v", lead)
	}
	if merged.ID != "518" || !merged.Closed || merged.URI != "https://kubcrm.kz/leads/539" || len(merged.ContactIDs) != 2 ||
		merged.Name != "Лид #518 — объединён с лидом #539" {
		t.Fatalf("merged lead deal must point to the main lead: %+v", merged)
	}
}

// Объединили лиды — обе переписки за ответственным, обе сделки обновлены.
func TestLeadsMergedPushesChatsAndDeals(t *testing.T) {
	svc, roles, sent := newRolesService(t)
	svc.accounts = svc.accounts[:1]
	svc.SetCRMBaseURL("https://kubcrm.kz")
	svc.repo = mergedLeadRepo{
		leadContactsRepo: &leadContactsRepo{rolesRepo: roles},
		chats: []repositories.ChatLeadContact{
			{Transport: "whatsapp", ChatID: "77775120593", Name: "Индира", OwnerID: userSales, LeadID: 539, LeadTitle: "Лид из WhatsApp"},
			{Transport: "instagram", ChatID: "indira", OwnerID: userSales, LeadID: 539, LeadTitle: "Лид из WhatsApp"},
		},
	}
	assigned, keys, users, deals := &[]ContactUpsert{}, &[]string{}, &[]UserUpsert{}, &[]DealUpsert{}
	svc.accounts[0].Client = contactsClient{rolesClient: rolesClient{sent: sent}, assigned: assigned, keys: keys, users: users, deals: deals}

	svc.LeadsMerged(context.Background(), 539, 518, userSales)
	if len(*assigned) != 2 || (*assigned)[0].ResponsibleUserID != wz(userSales) || (*assigned)[1].ResponsibleUserID != wz(userSales) {
		t.Fatalf("both chats must go to the lead owner: %+v", *assigned)
	}
	if len(*deals) != 2 || (*deals)[0].ID != "539" || len((*deals)[0].ContactIDs) != 2 ||
		(*deals)[1].ID != "518" || !(*deals)[1].Closed || (*deals)[1].URI != "https://kubcrm.kz/leads/539" {
		t.Fatalf("unexpected deals: %+v", *deals)
	}
}

// Взяли объединённый лид из чата WhatsApp — чат Instagram тоже переходит к
// сотруднику, сделка ссылается на оба.
func TestAssignChatOwnerCoversAllLeadChats(t *testing.T) {
	svc, roles, sent := newRolesService(t)
	roles.chatChannel = map[string]string{"77775120593": "main-almaty"}
	svc.repo = mergedLeadRepo{
		leadContactsRepo: &leadContactsRepo{rolesRepo: roles},
		chats: []repositories.ChatLeadContact{
			{Transport: "whatsapp", ChatID: "77775120593", LeadID: 539},
			{Transport: "instagram", ChatID: "indira", LeadID: 539},
		},
	}
	assigned, deals := &[]ContactUpsert{}, &[]DealUpsert{}
	svc.accounts[0].Client = contactsClient{rolesClient: rolesClient{sent: sent}, assigned: assigned, deals: deals}

	svc.AssignChatOwner(context.Background(), "whatsapp", "77775120593", userSales)
	if len(*assigned) != 2 || (*assigned)[1].ID != "kub-instagram-indira" {
		t.Fatalf("all chats of the lead must be assigned, got %+v", *assigned)
	}
	if len(*deals) != 1 || len((*deals)[0].ContactIDs) != 2 {
		t.Fatalf("deal must reference both chats, got %+v", *deals)
	}
}
