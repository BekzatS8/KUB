package meta

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc) (*Client, func()) {
	t.Helper()
	srv := httptest.NewServer(h)
	c := NewClient(Config{AppID: "app", AppSecret: "secret", RedirectURL: "https://crm/cb", GraphBaseURL: srv.URL}, 5*time.Second)
	return c, srv.Close
}

func TestDialogURL(t *testing.T) {
	if _, err := NewClient(Config{AppID: "app"}, 0).DialogURL("s"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("not configured client must refuse, got %v", err)
	}
	raw, err := NewClient(Config{AppID: "app", AppSecret: "secret", RedirectURL: "https://crm/cb"}, 0).DialogURL("st")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(raw)
	q := u.Query()
	if u.Host != "www.facebook.com" || u.Path != "/"+DefaultGraphVersion+"/dialog/oauth" ||
		q.Get("client_id") != "app" || q.Get("redirect_uri") != "https://crm/cb" || q.Get("state") != "st" ||
		!strings.Contains(q.Get("scope"), "instagram_manage_messages") || q.Get("client_secret") != "" {
		t.Fatalf("unexpected dialog url %s", raw)
	}
}

// code меняется на короткий токен, а тот — на долгоживущий.
func TestExchangeCodeReturnsLongLivedToken(t *testing.T) {
	c, closeFn := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("code") == "abc" && q.Get("redirect_uri") == "https://crm/cb" && q.Get("client_secret") == "secret":
			_, _ = io.WriteString(w, `{"access_token":"short","token_type":"bearer"}`)
		case q.Get("grant_type") == "fb_exchange_token" && q.Get("fb_exchange_token") == "short":
			_, _ = io.WriteString(w, `{"access_token":"long","expires_in":5183944}`)
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"bad request","code":100}}`)
		}
	})
	defer closeFn()
	tok, err := c.ExchangeCode(context.Background(), "abc")
	if err != nil || tok != "long" {
		t.Fatalf("expected long-lived token, got %q %v", tok, err)
	}
}

func TestExchangeCodeGraphError(t *testing.T) {
	c, closeFn := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"This authorization code has expired.","code":100}}`)
	})
	defer closeFn()
	_, err := c.ExchangeCode(context.Background(), "old")
	var ge *GraphError
	if !errors.As(err, &ge) || ge.Message != "This authorization code has expired." {
		t.Fatalf("graph refusal must keep its message, got %v", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("error must not leak app secret: %v", err)
	}
}

// Страницы без Instagram пропускаются, страницы листаются по курсору.
func TestInstagramAccountsFiltersAndPaginates(t *testing.T) {
	c, closeFn := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+DefaultGraphVersion+"/me/accounts" || r.URL.Query().Get("access_token") != "tok" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("after") == "" {
			_, _ = io.WriteString(w, `{"data":[
				{"id":"p1","name":"VISARIO","instagram_business_account":{"id":"i1","username":"visario.kz"}},
				{"id":"p2","name":"Без Instagram"}
			],"paging":{"cursors":{"after":"c1"},"next":"https://graph/next"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"p3","name":"Второй","instagram_business_account":{"id":"i3","username":"second"}}],"paging":{"cursors":{"after":"c2"}}}`)
	})
	defer closeFn()
	got, err := c.InstagramAccounts(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (InstagramAccount{PageID: "p1", PageName: "VISARIO", InstID: "i1", Username: "visario.kz"}) || got[1].InstID != "i3" {
		t.Fatalf("unexpected accounts: %+v", got)
	}
}

func TestVerifyApp(t *testing.T) {
	c, closeFn := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("grant_type") == "client_credentials" && q.Get("client_id") == "app" && q.Get("client_secret") == "secret" {
			_, _ = io.WriteString(w, `{"access_token":"app|token","token_type":"bearer"}`)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"Error validating client secret.","code":1}}`)
	})
	defer closeFn()
	if err := c.VerifyApp(context.Background()); err != nil {
		t.Fatalf("valid keys must pass, got %v", err)
	}
	bad := NewClient(Config{AppID: "app", AppSecret: "wrong", GraphBaseURL: c.graphBase}, time.Second)
	var ge *GraphError
	if err := bad.VerifyApp(context.Background()); !errors.As(err, &ge) || ge.Message != "Error validating client secret." {
		t.Fatalf("wrong secret must be reported with facebook message, got %v", err)
	}
}
