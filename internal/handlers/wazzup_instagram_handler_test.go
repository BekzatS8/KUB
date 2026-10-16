package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"turcompany/internal/integrations/meta"
)

func newInstagramTestHandler(svc *stubWazzupService) *WazzupHandler {
	h := NewWazzupHandler(svc)
	h.SetMeta(meta.NewClient(meta.Config{AppID: "app", AppSecret: "secret", RedirectURL: "https://crm/cb"}, 0))
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
	h.SetMeta(meta.NewClient(meta.Config{AppID: "app"}, 0))
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
