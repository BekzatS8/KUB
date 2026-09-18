package wazzup

import (
	"context"
	"errors"
	"testing"

	"turcompany/internal/models"
)

type authorClient struct {
	noopClient
	upserted []UserUpsert
	sent     []SendMessageRequest
	failWith error
	failOnce bool
}

func (c *authorClient) UpsertUsers(_ context.Context, _ string, users []UserUpsert) error {
	c.upserted = append(c.upserted, users...)
	return nil
}

func (c *authorClient) SendMessage(_ context.Context, _ string, req SendMessageRequest) (*SendMessageResponse, error) {
	c.sent = append(c.sent, req)
	if c.failWith != nil && (!c.failOnce || len(c.sent) == 1) {
		return nil, c.failWith
	}
	return &SendMessageResponse{MessageID: "ok"}, nil
}

func newAuthorService(client Client) *Service {
	return NewService(stubRepo{
		integration: &models.WazzupIntegration{ID: 1, OwnerUserID: 1, APIKeyEnc: "key", Enabled: true},
	}, client, "", "channel", "", "")
}

// Обратная связь заказчика 18.09.2026: менеджер визового отдела пишет клиенту,
// а в WhatsApp сообщение подписано «API • Admin». Автора нужно передавать
// провайдеру, и это должен быть тот, кто реально пишет.
func TestSendMessage_SendsAuthorOfTheActualSender(t *testing.T) {
	client := &authorClient{}
	if _, err := newAuthorService(client).SendMessage(context.Background(), 42, "77002700946", "whatsapp", "ch-1", "привет"); err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if len(client.sent) != 1 {
		t.Fatalf("expected one send, got %d", len(client.sent))
	}
	want := wazzupUserIDFor(0, 42)
	if client.sent[0].CRMUserID != want {
		t.Fatalf("expected author %q, got %q", want, client.sent[0].CRMUserID)
	}
	// Автор должен быть заранее зарегистрирован у провайдера, иначе он его не примет.
	if len(client.upserted) != 1 || client.upserted[0].ID != want {
		t.Fatalf("sender must be registered before sending, got %#v", client.upserted)
	}
}

// Подпись автора не повод не доставить сообщение: если провайдер не знает
// пользователя, повторяем отправку без автора.
func TestSendMessage_RetriesWithoutAuthorWhenProviderRejectsIt(t *testing.T) {
	client := &authorClient{failWith: errors.New("wazzup POST /v3/message failed: status=400 body={\"error\":\"crmUserId not found\"}"), failOnce: true}
	if _, err := newAuthorService(client).SendMessage(context.Background(), 42, "77002700946", "whatsapp", "ch-1", "привет"); err != nil {
		t.Fatalf("SendMessage must survive an unknown author: %v", err)
	}
	if len(client.sent) != 2 {
		t.Fatalf("expected a retry, got %d sends", len(client.sent))
	}
	if client.sent[1].CRMUserID != "" {
		t.Fatalf("retry must drop the author, got %q", client.sent[1].CRMUserID)
	}
}

// Прочие ошибки провайдера повторять нельзя — иначе клиент получит дубль.
func TestSendMessage_DoesNotRetryUnrelatedErrors(t *testing.T) {
	client := &authorClient{failWith: errors.New("wazzup POST /v3/message failed: status=402 body={\"error\":\"no money\"}")}
	if _, err := newAuthorService(client).SendMessage(context.Background(), 42, "77002700946", "whatsapp", "ch-1", "привет"); err == nil {
		t.Fatal("expected the upstream error to surface")
	}
	if len(client.sent) != 1 {
		t.Fatalf("expected exactly one send attempt, got %d", len(client.sent))
	}
}

// Один и тот же сотрудник должен иметь один id и в iframe, и при отправке,
// иначе в Wazzup он задваивается.
func TestWazzupUserIDFor(t *testing.T) {
	if got := wazzupUserIDFor(0, 42); got != "kub-42-42" {
		t.Fatalf("unexpected id without company: %q", got)
	}
	if got := wazzupUserIDFor(7, 42); got != "kub-7-42" {
		t.Fatalf("unexpected id with company: %q", got)
	}
	if got := wazzupUserIDFor(0, 0); got != "" {
		t.Fatalf("missing user must produce an empty id, got %q", got)
	}
}
