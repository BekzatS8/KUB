package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type linksRepoStub struct {
	link            *repositories.TelegramLink
	confirmChatID   int64
	confirmErr      error
	getByCodeCalls  int
	lastGetByCode   string
	lastConfirmCode string
	lastConfirmUser int
}

func (s *linksRepoStub) CreateLink(ctx context.Context, userID int, chatID int64, code string, expiresAt time.Time) (*repositories.TelegramLink, error) {
	return nil, nil
}
func (s *linksRepoStub) GetByCode(ctx context.Context, code string) (*repositories.TelegramLink, error) {
	s.getByCodeCalls++
	s.lastGetByCode = code
	return s.link, nil
}
func (s *linksRepoStub) AttachChatID(ctx context.Context, code string, chatID int64) error {
	return nil
}
func (s *linksRepoStub) ConfirmLink(ctx context.Context, code string, userID int) (int64, error) {
	s.lastConfirmCode = code
	s.lastConfirmUser = userID
	return s.confirmChatID, s.confirmErr
}

type usersRepoStub struct{ updateCalled bool }

func (u *usersRepoStub) Create(user *models.User) error                       { return nil }
func (u *usersRepoStub) GetByID(id int) (*models.User, error)                 { return nil, nil }
func (u *usersRepoStub) Update(user *models.User) error                       { return nil }
func (u *usersRepoStub) Delete(id int) error                                  { return nil }
func (u *usersRepoStub) List(limit, offset int) ([]*models.User, error)       { return nil, nil }
func (u *usersRepoStub) GetByEmail(email string) (*models.User, error)        { return nil, nil }
func (u *usersRepoStub) GetCount() (int, error)                               { return 0, nil }
func (u *usersRepoStub) GetCountByRole(roleID int) (int, error)               { return 0, nil }
func (u *usersRepoStub) UpdatePassword(userID int, passwordHash string) error { return nil }
func (u *usersRepoStub) UpdateRefresh(userID int, token string, expiresAt time.Time) error {
	return nil
}
func (u *usersRepoStub) RotateRefresh(oldToken, newToken string, newExpiresAt time.Time) (*models.User, error) {
	return nil, nil
}
func (u *usersRepoStub) ClearRefresh(userID int) error                        { return nil }
func (u *usersRepoStub) GetByRefreshToken(token string) (*models.User, error) { return nil, nil }
func (u *usersRepoStub) VerifyUser(userID int) error                          { return nil }
func (u *usersRepoStub) UpdateTelegramLink(userID int, chatID int64, enable bool) error {
	u.updateCalled = true
	return nil
}
func (u *usersRepoStub) GetByIDSimple(id int) (*models.User, error) { return nil, nil }
func (u *usersRepoStub) GetTelegramSettings(ctx context.Context, userID int64) (chatID int64, notify bool, err error) {
	return 0, false, nil
}
func (u *usersRepoStub) GetByChatID(ctx context.Context, chatID int64) (*models.User, error) {
	return nil, nil
}

func performConfirmLink(t *testing.T, h *IntegrationsHandler, code string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/integrations/telegram/link", func(c *gin.Context) {
		c.Set("user_id", 42)
		h.ConfirmLink(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/integrations/telegram/link?code="+code, nil)
	r.ServeHTTP(w, req)

	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w, body
}

func TestConfirmLink_NotFound(t *testing.T) {
	h := &IntegrationsHandler{LinksRepo: &linksRepoStub{}, UsersRepo: &usersRepoStub{}}
	w, body := performConfirmLink(t, h, "abc")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if body["message"] != "invalid or expired code" {
		t.Fatalf("unexpected message: %#v", body)
	}
}

func TestConfirmLink_Expired(t *testing.T) {
	h := &IntegrationsHandler{LinksRepo: &linksRepoStub{link: &repositories.TelegramLink{Code: "ABC", ExpiresAt: time.Now().UTC().Add(-time.Minute)}}, UsersRepo: &usersRepoStub{}}
	w, _ := performConfirmLink(t, h, "abc")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestConfirmLink_ChatNotAttached(t *testing.T) {
	h := &IntegrationsHandler{LinksRepo: &linksRepoStub{
		link:       &repositories.TelegramLink{Code: "ABC", ExpiresAt: time.Now().UTC().Add(time.Minute)},
		confirmErr: repositories.ErrTelegramChatNotAttached,
	}, UsersRepo: &usersRepoStub{}}
	w, body := performConfirmLink(t, h, "abc")
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
	if body["error"] != "telegram chat not attached" {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestConfirmLink_Success(t *testing.T) {
	users := &usersRepoStub{}
	links := &linksRepoStub{
		link:          &repositories.TelegramLink{Code: "ABC", ExpiresAt: time.Now().UTC().Add(time.Minute), ChatID: sql.NullInt64{Int64: 999, Valid: true}},
		confirmChatID: 999,
	}
	h := &IntegrationsHandler{LinksRepo: links, UsersRepo: users}
	w, body := performConfirmLink(t, h, "abc")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body["status"] != "ok" {
		t.Fatalf("unexpected body: %#v", body)
	}
	if links.lastConfirmCode != "ABC" {
		t.Fatalf("expected uppercased code, got %s", links.lastConfirmCode)
	}
	if !users.updateCalled {
		t.Fatalf("expected user telegram link update")
	}
}
