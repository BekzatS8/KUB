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
		rows = append(rows, repositories.ChatLeadContact{Transport: "whatsapp", ChatID: fmt.Sprintf("7700%07d", i), OwnerID: owner, OwnerName: "Менеджер"})
	}
	rows[0].Name = "Айгерим"
	rows[1] = repositories.ChatLeadContact{Transport: "instagram", ChatID: "happy_client", OwnerID: userSales2, OwnerName: "Кулжанова"}
	repo := &leadContactsRepo{rolesRepo: roles, rows: rows}
	svc.repo = repo

	assigned, keys, users := &[]ContactUpsert{}, &[]string{}, &[]UserUpsert{}
	svc.accounts[0].Client = contactsClient{rolesClient: rolesClient{sent: sent}, assigned: assigned, keys: keys, users: users}
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
}
