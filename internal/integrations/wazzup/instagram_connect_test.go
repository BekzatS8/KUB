package wazzup

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"turcompany/internal/models"
)

// Канал создаётся методом POST /v2/channels с transport=instagram и пустыми
// учётными данными, а ссылка авторизации берётся из поля url.
func TestPartnerClientCreateChannelReturnsAuthURL(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	client, _, closeFn := newTestPartnerClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":[{"channel_id":"ig-1","transport":"instagram","state":"init","phone":"","username":"","messenger_id":"","name":"","url":"https://www.facebook.com/dialog/oauth?x=1"}],"meta":{}}`)
	})
	defer closeFn()

	created, err := client.CreateChannel(context.Background(), "instagram")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v2/channels" {
		t.Fatalf("unexpected request %s %s", gotMethod, gotPath)
	}
	if gotBody["transport"] != "instagram" {
		t.Fatalf("transport must be instagram, got %v", gotBody["transport"])
	}
	if creds, ok := gotBody["credentials"].(map[string]any); !ok || len(creds) != 0 {
		t.Fatalf("credentials must be an empty object, got %v", gotBody["credentials"])
	}
	if created.Channel.ID != "ig-1" || created.Channel.Status != "init" || created.AuthURL != "https://www.facebook.com/dialog/oauth?x=1" {
		t.Fatalf("unexpected result: %+v", created)
	}
}

// Ответ может прийти объектом, а ссылка — в details.initUrl (формат v1).
func TestParseCreatedChannelVariants(t *testing.T) {
	c, err := parseCreatedChannel([]byte(`{"data":{"channel_id":"ig-2","state":"init","details":{"initUrl":"https://fb/init"}}}`))
	if err != nil || c.Channel.ID != "ig-2" || c.AuthURL != "https://fb/init" {
		t.Fatalf("object response: %+v %v", c, err)
	}
	c, err = parseCreatedChannel([]byte(`{"data":[],"meta":{}}`))
	if err != nil || c.AuthURL != "" || len(c.Raw) == 0 {
		t.Fatalf("empty response must keep raw body for diagnostics: %+v %v", c, err)
	}
}

// Отказ Wazzup показывается администратору его же словами.
func TestProviderErrorDetail(t *testing.T) {
	err := errors.New(`wazzup partner POST /v2/channels failed: status=400 body={"code":"VALIDATION_FAILED","title":"Validation Failed","detail":"One or more fields did not pass validation.","errors":["account limit exceeded"]}`)
	got := providerErrorDetail(err)
	if !strings.Contains(got, "One or more fields") || !strings.Contains(got, "account limit exceeded") {
		t.Fatalf("detail must include provider explanation, got %q", got)
	}
}

// instagramClient — клиент дочернего аккаунта, создающий канал.
type instagramClient struct {
	noopClient
	created *CreatedChannel
	err     error
}

func (c instagramClient) CreateChannel(context.Context, string) (*CreatedChannel, error) {
	return c.created, c.err
}

func TestConnectInstagramUsesPartnerAccount(t *testing.T) {
	repo := &multiRepo{
		integrations: map[string]*models.WazzupIntegration{
			AccountMain:  {ID: 1, Enabled: true, Account: AccountMain},
			AccountChild: {ID: 2, Enabled: true, Account: AccountChild},
		},
		channels: map[int][]models.WazzupChannel{},
	}
	svc := NewService(repo, recClient{name: "main", calls: &[]string{}}, "main-key", "", "", "")
	svc.RegisterAccount(AccountConfig{Name: AccountChild, Partner: true, Client: instagramClient{
		created: &CreatedChannel{Channel: Channel{ID: "ig-1", Status: "init"}, AuthURL: "https://fb/auth"},
	}})

	res, err := svc.ConnectInstagram(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Account != AccountChild || res.ChannelID != "ig-1" || res.AuthURL != "https://fb/auth" || res.ProviderResponse != "" {
		t.Fatalf("unexpected result: %+v", res)
	}

	svc.accounts[1].Client = instagramClient{err: errors.New(`wazzup partner POST /v2/channels failed: status=400 body={"detail":"channel creation failed"}`)}
	_, err = svc.ConnectInstagram(context.Background())
	var createErr *ChannelCreateError
	if !errors.As(err, &createErr) || createErr.Detail != "channel creation failed" || !errors.Is(err, ErrUpstream) {
		t.Fatalf("provider refusal must become ChannelCreateError with its detail, got %v", err)
	}
}
