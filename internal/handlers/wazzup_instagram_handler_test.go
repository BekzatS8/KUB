package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"turcompany/internal/integrations/meta"
	"turcompany/internal/repositories"
)

// fakeMetaSettings — ключи приложения Meta «в базе».
type fakeMetaSettings struct {
	saved repositories.MetaAppSettings
	saves int
}

func (f *fakeMetaSettings) Get(context.Context) (*repositories.MetaAppSettings, error) {
	s := f.saved
	return &s, nil
}

func (f *fakeMetaSettings) Save(_ context.Context, appID, appSecret string, _ int) error {
	f.saves++
	f.saved.AppID = appID
	if appSecret != "" {
		f.saved.AppSecret = appSecret
	}
	return nil
}

func newInstagramTestHandler(svc *stubWazzupService) *WazzupHandler {
	h := NewWazzupHandler(svc)
	h.SetMeta(&fakeMetaSettings{saved: repositories.MetaAppSettings{AppID: "123", AppSecret: "secret"}},
		meta.Config{RedirectURL: "https://crm/cb"}, 0)
	return h
}

func serveAs(h gin.HandlerFunc, userID, roleID int, method, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Handle(method, "/x", func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Set("role_id", roleID)
		h(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// state одноразовый и принадлежит тому, кто начал вход.
func TestInstagramLoginStateOwnership(t *testing.T) {
	s := newInstagramLoginStore()
	st, err := s.newState(1)
	if err != nil {
		t.Fatal(err)
	}
	if s.takeState(st, 2) {
		t.Fatal("state of another user must be rejected")
	}
	if !s.takeState(st, 1) {
		t.Fatal("owner must be able to use state")
	}
	if s.takeState(st, 1) {
		t.Fatal("state must be single-use")
	}
}

func TestInstagramLoginNeedsMetaApp(t *testing.T) {
	h := NewWazzupHandler(&stubWazzupService{})
	h.SetMeta(&fakeMetaSettings{}, meta.Config{RedirectURL: "https://crm/cb"}, 0)
	w := serveAs(h.InstagramLogin, 1, 50, http.MethodPost, `{}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "meta_app_not_configured") {
		t.Fatalf("expected meta_app_not_configured, got %d %s", w.Code, w.Body.String())
	}
	if w := serveAs(newInstagramTestHandler(&stubWazzupService{}).InstagramLogin, 1, 10, http.MethodPost, `{}`); w.Code != http.StatusForbidden {
		t.Fatalf("non-admin must be forbidden, got %d", w.Code)
	}
}

func TestInstagramLoginReturnsDialogURL(t *testing.T) {
	w := serveAs(newInstagramTestHandler(&stubWazzupService{}).InstagramLogin, 1, 50, http.MethodPost, `{}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "facebook.com") || !strings.Contains(w.Body.String(), "state=") {
		t.Fatalf("expected facebook dialog url, got %d %s", w.Code, w.Body.String())
	}
}

// Канал создаётся по аккаунту из сеанса входа, с токеном из сеанса —
// фронт присылает только session и inst_id.
func TestConnectInstagramUsesSessionToken(t *testing.T) {
	svc := &stubWazzupService{}
	h := newInstagramTestHandler(svc)
	session, err := h.igLogins.newSession(1, "fb-token", []meta.InstagramAccount{
		{PageID: "p1", PageName: "VISARIO", InstID: "i1", Username: "visario.kz"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if w := serveAs(h.ConnectInstagram, 2, 50, http.MethodPost, `{"session":"`+session+`","inst_id":"i1"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("session of another user must be rejected, got %d", w.Code)
	}
	if w := serveAs(h.ConnectInstagram, 1, 50, http.MethodPost, `{"session":"`+session+`","inst_id":"other"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown instagram account must be rejected, got %d", w.Code)
	}
	if svc.igCreds != nil {
		t.Fatal("provider must not be called for rejected requests")
	}

	w := serveAs(h.ConnectInstagram, 1, 50, http.MethodPost, `{"session":"`+session+`","inst_id":"i1"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", w.Code, w.Body.String())
	}
	if svc.igCreds == nil || svc.igCreds.UserToken != "fb-token" || svc.igCreds.PageID != "p1" || svc.igCreds.Username != "visario.kz" {
		t.Fatalf("unexpected credentials: %+v", svc.igCreds)
	}
	if strings.Contains(w.Body.String(), "fb-token") {
		t.Fatal("facebook token must not be returned to the client")
	}
	if _, ok := h.igLogins.session(session, 1); ok {
		t.Fatal("session must be dropped after the channel is created")
	}
}

// Ключи приложения вводятся в CRM: сначала проверяются в Facebook, секрет на
// фронт не возвращается, пустой секрет при правке оставляет прежний.
func TestSaveInstagramSettingsVerifiesAndHidesSecret(t *testing.T) {
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("grant_type") == "client_credentials" && q.Get("client_id") == "555" && q.Get("client_secret") == "good" {
			_, _ = io.WriteString(w, `{"access_token":"app|t"}`)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"Error validating client secret.","code":1}}`)
	}))
	defer graph.Close()

	repo := &fakeMetaSettings{}
	h := NewWazzupHandler(&stubWazzupService{})
	h.SetMeta(repo, meta.Config{RedirectURL: "https://crm/cb", GraphBaseURL: graph.URL}, 0)

	w := serveAs(h.InstagramSettings, 1, 50, http.MethodGet, ``)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"configured":false`) || !strings.Contains(w.Body.String(), "https://crm/cb") {
		t.Fatalf("empty settings must show redirect url and configured=false, got %d %s", w.Code, w.Body.String())
	}
	if w := serveAs(h.SaveInstagramSettings, 1, 50, http.MethodPut, `{"app_id":"abc","app_secret":"good"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("non-numeric app id must be rejected, got %d", w.Code)
	}
	w = serveAs(h.SaveInstagramSettings, 1, 50, http.MethodPut, `{"app_id":"555","app_secret":"bad"}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Error validating client secret.") || repo.saves != 0 {
		t.Fatalf("wrong keys must not be saved, got %d %s saves=%d", w.Code, w.Body.String(), repo.saves)
	}
	w = serveAs(h.SaveInstagramSettings, 1, 50, http.MethodPut, `{"app_id":"555","app_secret":"good"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"configured":true`) || strings.Contains(w.Body.String(), "good") {
		t.Fatalf("valid keys must be saved without echoing secret, got %d %s", w.Code, w.Body.String())
	}
	if repo.saved.AppSecret != "good" {
		t.Fatalf("secret must be stored, got %+v", repo.saved)
	}
	// Правка без секрета — секрет прежний, проверка идёт с ним.
	w = serveAs(h.SaveInstagramSettings, 1, 50, http.MethodPut, `{"app_id":"555","app_secret":""}`)
	if w.Code != http.StatusOK || repo.saved.AppSecret != "good" {
		t.Fatalf("empty secret must keep the stored one, got %d %s %+v", w.Code, w.Body.String(), repo.saved)
	}
	if w := serveAs(h.SaveInstagramSettings, 1, 10, http.MethodPut, `{"app_id":"555","app_secret":"good"}`); w.Code != http.StatusForbidden {
		t.Fatalf("non-admin must be forbidden, got %d", w.Code)
	}
}

// Ключи, введённые в CRM, важнее ключей из переменных сервера.
func TestMetaSourcePrefersCRMKeys(t *testing.T) {
	src := &metaSource{repo: &fakeMetaSettings{}, base: meta.Config{AppID: "1", AppSecret: "srv", RedirectURL: "https://crm/cb"}}
	c, source, err := src.client(context.Background())
	if err != nil || source != "server" || c.AppID() != "1" {
		t.Fatalf("server keys must be the fallback, got %q %q %v", source, c.AppID(), err)
	}
	src.repo = &fakeMetaSettings{saved: repositories.MetaAppSettings{AppID: "2", AppSecret: "crm"}}
	c, source, _ = src.client(context.Background())
	if source != "crm" || c.AppID() != "2" {
		t.Fatalf("crm keys must win, got %q %q", source, c.AppID())
	}
}
