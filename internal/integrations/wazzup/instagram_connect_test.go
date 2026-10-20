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

// Канал Instagram создаётся методом POST /v2/channels с transport=instagram и
// данными из входа через Facebook в credentials.
func TestPartnerClientCreateChannelSendsCredentials(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	client, _, closeFn := newTestPartnerClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":[{"channel_id":"ig-1","transport":"instagram","state":"init"}],"meta":{}}`)
	})
	defer closeFn()

	created, err := client.CreateChannel(context.Background(), "instagram", map[string]any{
		"username": "visario", "page_id": "p1", "inst_id": "i1", "user_token": "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v2/channels" {
		t.Fatalf("unexpected request %s %s", gotMethod, gotPath)
	}
	if gotBody["transport"] != "instagram" {
		t.Fatalf("transport must be instagram, got %v", gotBody["transport"])
	}
	creds, _ := gotBody["credentials"].(map[string]any)
	if creds["username"] != "visario" || creds["page_id"] != "p1" || creds["inst_id"] != "i1" || creds["user_token"] != "tok" {
		t.Fatalf("credentials must carry facebook login data, got %v", gotBody["credentials"])
	}
	if created.Channel.ID != "ig-1" || created.Channel.Status != "init" {
		t.Fatalf("unexpected result: %+v", created)
	}
}

// Ответ может прийти объектом, а не списком.
func TestParseCreatedChannelVariants(t *testing.T) {
	c, err := parseCreatedChannel([]byte(`{"data":{"channel_id":"ig-2","state":"init"}}`))
	if err != nil || c.Channel.ID != "ig-2" {
		t.Fatalf("object response: %+v %v", c, err)
	}
	c, err = parseCreatedChannel([]byte(`{"data":[],"meta":{}}`))
	if err != nil || c.Channel.ID != "" || len(c.Raw) == 0 {
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
	got     *map[string]any
}

func (c instagramClient) CreateChannel(_ context.Context, _ string, creds map[string]any) (*CreatedChannel, error) {
	if c.got != nil {
		*c.got = creds
	}
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
	var got map[string]any
	svc.RegisterAccount(AccountConfig{Name: AccountChild, Partner: true, Client: instagramClient{
		created: &CreatedChannel{Channel: Channel{ID: "ig-1", Status: "init"}},
		got:     &got,
	}})
	creds := InstagramCredentials{Username: "@visario", PageID: "p1", InstID: "i1", UserToken: "tok"}

	res, err := svc.ConnectInstagram(context.Background(), creds)
	if err != nil {
		t.Fatal(err)
	}
	if res.Account != AccountChild || res.ChannelID != "ig-1" || res.Username != "visario" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if got["username"] != "visario" || got["user_token"] != "tok" {
		t.Fatalf("credentials must reach provider without @, got %v", got)
	}

	if _, err := svc.ConnectInstagram(context.Background(), InstagramCredentials{Username: "visario"}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("incomplete credentials must be rejected before provider call, got %v", err)
	}

	svc.accounts[1].Client = instagramClient{err: errors.New(`wazzup partner POST /v2/channels failed: status=400 body={"detail":"channel creation failed"}`)}
	_, err = svc.ConnectInstagram(context.Background(), creds)
	var createErr *ChannelCreateError
	if !errors.As(err, &createErr) || createErr.Detail != "channel creation failed" || !errors.Is(err, ErrUpstream) {
		t.Fatalf("provider refusal must become ChannelCreateError with its detail, got %v", err)
	}
}

// seqInstagramClient — отвечает по очереди и запоминает присланные credentials.
type seqInstagramClient struct {
	noopClient
	calls *[]map[string]any
	errs  []error
	out   *CreatedChannel
}

func (c seqInstagramClient) CreateChannel(_ context.Context, _ string, creds map[string]any) (*CreatedChannel, error) {
	n := len(*c.calls)
	*c.calls = append(*c.calls, creds)
	if n < len(c.errs) && c.errs[n] != nil {
		return nil, c.errs[n]
	}
	return c.out, nil
}

// Боевой Wazzup отвергает поля из документации («should not exist») — CRM
// создаёт канал с пустыми credentials и отдаёт ссылку авторизации Wazzup.
func TestConnectInstagramFallsBackToEmptyCredentials(t *testing.T) {
	repo := &multiRepo{
		integrations: map[string]*models.WazzupIntegration{AccountChild: {ID: 2, Enabled: true, Account: AccountChild}},
		channels:     map[int][]models.WazzupChannel{},
	}
	svc := NewService(repo, nil, "", "", "", "")
	calls := &[]map[string]any{}
	svc.RegisterAccount(AccountConfig{Name: AccountChild, Partner: true, Client: seqInstagramClient{
		calls: calls,
		errs:  []error{errors.New(`wazzup partner POST /v2/channels failed: status=400 body={"detail":"One or more fields did not pass validation.","errors":["credentials.inst_id: property inst_id should not exist"]}`)},
		out:   &CreatedChannel{Channel: Channel{ID: "ig-1", Status: "init"}, AuthURL: "https://app.wazzup24.com/fb-auth"},
	}})
	res, err := svc.ConnectInstagram(context.Background(), InstagramCredentials{Username: "visario.kz", PageID: "p", InstID: "i", UserToken: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 || (*calls)[0]["inst_id"] != "i" || len((*calls)[1]) != 0 {
		t.Fatalf("expected documented credentials, then empty ones, got %v", *calls)
	}
	if res.AuthURL != "https://app.wazzup24.com/fb-auth" || res.ChannelID != "ig-1" {
		t.Fatalf("auth url from wazzup must reach the admin, got %+v", res)
	}
}
