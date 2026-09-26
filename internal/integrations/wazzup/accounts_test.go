package wazzup

import (
	"context"
	"errors"
	"testing"

	"turcompany/internal/models"
)

// multiRepo — два подключения (основной и дочерний аккаунты) со своими
// номерами. Всё остальное — от stubRepo.
type multiRepo struct {
	stubRepo
	integrations map[string]*models.WazzupIntegration // account → подключение
	channels     map[int][]models.WazzupChannel       // integration id → номера
	chatChannel  map[string]string                    // chat id → external channel id
	upserts      []upsertCall
}

type upsertCall struct {
	account, crmHash, webhooksURI string
}

func (r *multiRepo) GetIntegrationByAccount(_ context.Context, account string) (*models.WazzupIntegration, error) {
	return r.integrations[account], nil
}
func (r *multiRepo) ListChannels(_ context.Context, integrationID int) ([]models.WazzupChannel, error) {
	return r.channels[integrationID], nil
}
func (r *multiRepo) GetChatChannelID(_ context.Context, _ string, chatID string) (string, error) {
	return r.chatChannel[chatID], nil
}
func (r *multiRepo) UpsertIntegrationByAccount(_ context.Context, account string, _ int, _ string, crmHash, webhooksURI string, _ bool) (int, string, error) {
	r.upserts = append(r.upserts, upsertCall{account: account, crmHash: crmHash, webhooksURI: webhooksURI})
	return 1, "tok-" + account, nil
}
func (r *multiRepo) UpsertChannels(context.Context, int, []models.WazzupChannel) error { return nil }
func (r *multiRepo) DeleteChannelsNotIn(context.Context, int, []string) (int64, error) {
	return 0, nil
}

// recClient — клиент аккаунта, запоминающий, что через него вызывали.
type recClient struct {
	noopClient
	name     string
	calls    *[]string
	patchErr error
	channels []Channel
}

func (c recClient) note(op string) { *c.calls = append(*c.calls, c.name+":"+op) }

func (c recClient) SendMessage(_ context.Context, _ string, req SendMessageRequest) (*SendMessageResponse, error) {
	c.note("send:" + req.ChannelID)
	return &SendMessageResponse{MessageID: c.name + "-msg"}, nil
}
func (c recClient) CreateIframe(context.Context, string, CreateIframeRequest) (string, error) {
	c.note("iframe")
	return "https://iframe/" + c.name, nil
}
func (c recClient) PatchWebhooks(context.Context, string, string, string) error {
	c.note("webhooks")
	return c.patchErr
}
func (c recClient) ListChannels(context.Context, string) ([]Channel, error) {
	c.note("list")
	return c.channels, nil
}

func newTwoAccountService(t *testing.T) (*Service, *multiRepo, *[]string) {
	t.Helper()
	calls := &[]string{}
	repo := &multiRepo{
		integrations: map[string]*models.WazzupIntegration{
			AccountMain:  {ID: 1, Enabled: true, Account: AccountMain, CRMKeyHash: "main-hash"},
			AccountChild: {ID: 2, Enabled: true, Account: AccountChild},
		},
		channels: map[int][]models.WazzupChannel{
			1: {{ID: 11, IntegrationID: 1, ExternalChannelID: "main-almaty", Transport: "whatsapp"}},
			2: {{ID: 21, IntegrationID: 2, ExternalChannelID: "child-shymkent", Transport: "whatsapp"}},
		},
		chatChannel: map[string]string{"77001112233": "child-shymkent"},
	}
	svc := NewService(repo, recClient{name: "main", calls: calls}, "main-key", "", "", "")
	svc.RegisterAccount(AccountConfig{Name: AccountChild, Title: "Дочерний", Client: recClient{name: "child", calls: calls}, Partner: true})
	return svc, repo, calls
}

func lastCall(calls *[]string) string {
	if len(*calls) == 0 {
		return ""
	}
	return (*calls)[len(*calls)-1]
}

// Отправка идёт через аккаунт номера, с которого пишут: чужой аккаунт о таком
// канале не знает и отклонит отправку.
func TestSendMessageRoutesByChannelAccount(t *testing.T) {
	svc, _, calls := newTwoAccountService(t)

	if _, err := svc.SendMessage(context.Background(), 5, "77009998877", "whatsapp", "child-shymkent", "привет"); err != nil {
		t.Fatal(err)
	}
	if got := lastCall(calls); got != "child:send:child-shymkent" {
		t.Fatalf("child channel must be sent via child account, got %q", got)
	}
	if _, err := svc.SendMessage(context.Background(), 5, "77009998877", "whatsapp", "main-almaty", "привет"); err != nil {
		t.Fatal(err)
	}
	if got := lastCall(calls); got != "main:send:main-almaty" {
		t.Fatalf("main channel must be sent via main account, got %q", got)
	}
}

func TestGetIframeOpensRequestedAccount(t *testing.T) {
	svc, _, calls := newTwoAccountService(t)

	resp, err := svc.GetIframe(context.Background(), 5, 5, "Менеджер", IframeOptions{Account: AccountChild})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Account != AccountChild || lastCall(calls) != "child:iframe" {
		t.Fatalf("child messenger must open via child account: resp=%+v last=%q", resp, lastCall(calls))
	}
}

// Переход на переписку из карточки клиента открывается в аккаунте номера этой
// переписки — даже если пришли из пункта меню другого аккаунта.
func TestGetIframeDeepLinkFollowsChatAccount(t *testing.T) {
	svc, _, calls := newTwoAccountService(t)

	resp, err := svc.GetIframe(context.Background(), 5, 5, "Менеджер", IframeOptions{
		Account: AccountMain, Transport: "whatsapp", ChatID: "77001112233",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Account != AccountChild || lastCall(calls) != "child:iframe" {
		t.Fatalf("chat of child number must open in child account: resp=%+v last=%q", resp, lastCall(calls))
	}
}

// Список номеров — из обоих аккаунтов, с пометкой, чей номер.
func TestSyncChannelsMergesAccounts(t *testing.T) {
	svc, _, _ := newTwoAccountService(t)
	svc.accounts[0].Client = recClient{name: "main", calls: &[]string{}, channels: []Channel{{ID: "main-almaty", Transport: "whatsapp"}}}
	svc.accounts[1].Client = recClient{name: "child", calls: &[]string{}, channels: []Channel{{ID: "child-shymkent", Transport: "whatsapp"}}}

	channels, err := svc.SyncChannels(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	byAccount := map[string]string{}
	for _, ch := range channels {
		byAccount[ch.ExternalChannelID] = ch.Account
	}
	if byAccount["main-almaty"] != AccountMain || byAccount["child-shymkent"] != AccountChild {
		t.Fatalf("channels must be tagged with their account: %+v", byAccount)
	}
}

// Неподключённый аккаунт — понятная ошибка, а не «интеграция не найдена».
func TestNotConnectedAccountError(t *testing.T) {
	svc, repo, _ := newTwoAccountService(t)
	delete(repo.integrations, AccountChild)

	_, err := svc.GetIframe(context.Background(), 5, 5, "Менеджер", IframeOptions{Account: AccountChild})
	if !errors.Is(err, ErrAccountNotConnected) {
		t.Fatalf("expected ErrAccountNotConnected, got %v", err)
	}
	// Совместимость: старые обработчики ловят ErrNotFound.
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ErrAccountNotConnected must also match ErrNotFound")
	}
}

// Новый crmKey сохраняется только после того, как провайдер принял вебхук.
// Иначе неудачная регистрация оставила бы базу и Wazzup с разными ключами, и
// входящие начали бы отклоняться.
func TestSetupKeepsOldKeyWhenProviderRejects(t *testing.T) {
	svc, repo, _ := newTwoAccountService(t)
	svc.accounts[0].Client = recClient{name: "main", calls: &[]string{}, patchErr: errors.New("provider down")}

	_, err := svc.Setup(context.Background(), 5, AccountMain, "https://api.example", true)
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("expected ErrUpstream, got %v", err)
	}
	for _, u := range repo.upserts {
		if u.crmHash != "main-hash" {
			t.Fatalf("stored crm hash must stay the old one when provider rejects, got %+v", repo.upserts)
		}
	}
}

func TestSetupSavesNewKeyAfterProviderAccepts(t *testing.T) {
	svc, repo, calls := newTwoAccountService(t)

	resp, err := svc.Setup(context.Background(), 5, AccountChild, "https://api.example", true)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Account != AccountChild || resp.WebhookURL != "https://api.example/integrations/wazzup/webhook/tok-child" {
		t.Fatalf("unexpected setup response: %+v", resp)
	}
	if lastCall(calls) != "child:webhooks" {
		t.Fatalf("webhooks must be registered in child account, got %q", lastCall(calls))
	}
	last := repo.upserts[len(repo.upserts)-1]
	if last.webhooksURI == "" || last.crmHash == "" {
		t.Fatalf("final upsert must store uri and new key hash: %+v", last)
	}
}

func TestRegisterAccountKeepsMainFirst(t *testing.T) {
	svc := NewService(stubRepo{}, nil, "", "", "", "")
	svc.RegisterAccount(AccountConfig{Name: AccountChild, Client: noopClient{}, Partner: true})
	svc.RegisterAccount(AccountConfig{Name: AccountMain, Client: noopClient{}})
	if svc.accounts[0].Name != AccountMain {
		t.Fatalf("main account must be the default (first), got %q", svc.accounts[0].Name)
	}
	// Без основного аккаунта по умолчанию — дочерний.
	only := NewService(stubRepo{}, nil, "", "", "", "")
	only.RegisterAccount(AccountConfig{Name: AccountChild, Client: noopClient{}, Partner: true})
	if acc, err := only.accountByName(""); err != nil || acc.Name != AccountChild {
		t.Fatalf("child must be default when main is absent: %v %v", acc, err)
	}
}
