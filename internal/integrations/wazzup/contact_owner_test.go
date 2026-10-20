package wazzup

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// contactsClient — клиент дочернего аккаунта, запоминающий назначения.
type contactsClient struct {
	rolesClient
	assigned *[]ContactUpsert
}

func (c contactsClient) UpsertContacts(_ context.Context, contacts []ContactUpsert) error {
	*c.assigned = append(*c.assigned, contacts...)
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
		got.ID != "kub-whatsapp-77001112233" || got.URI != "https://kubcrm.kz/whatsapp?transport=whatsapp&chat_id=77001112233" {
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

	err := client.UpsertContacts(context.Background(), []ContactUpsert{{
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
	// Основной аккаунт со своим кабинетом и незнакомые переписки не трогаем.
	svc.AssignChatOwner(context.Background(), "whatsapp", "77005556677", userSales2)
	svc.AssignChatOwner(context.Background(), "whatsapp", "77000000000", userSales2)
	if len(*assigned) != 1 {
		t.Fatalf("main account and unknown chats must stay untouched, got %+v", *assigned)
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
