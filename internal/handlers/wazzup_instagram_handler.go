package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/authz"
	"turcompany/internal/integrations/meta"
	wz "turcompany/internal/integrations/wazzup"
)

// Подключение Instagram к VISARIO (дочерний аккаунт Wazzup):
//
//  1. POST /integrations/wazzup/instagram/login — ссылка входа Facebook;
//  2. Facebook возвращает администратора на страницу CRM с code и state;
//  3. POST /integrations/wazzup/instagram/accounts {code, state} — CRM меняет
//     code на токен и отдаёт страницы с бизнес-аккаунтами Instagram;
//  4. POST /integrations/wazzup/channels/instagram {session, inst_id} — CRM
//     создаёт канал в Wazzup с данными выбранного аккаунта.
//
// Токен Facebook на фронт не уходит: он живёт в сеансе входа на сервере.

const instagramLoginTTL = 15 * time.Minute

type instagramLogin struct {
	userID    int
	token     string
	accounts  []meta.InstagramAccount
	expiresAt time.Time
}

// instagramLoginStore — незавершённые входы: state (до возврата из Facebook)
// и сеансы с токеном (до выбора аккаунта). Один процесс бэкенда — памяти
// достаточно; при перезапуске администратор просто войдёт ещё раз.
type instagramLoginStore struct {
	mu       sync.Mutex
	states   map[string]instagramLogin
	sessions map[string]instagramLogin
}

func newInstagramLoginStore() *instagramLoginStore {
	return &instagramLoginStore{states: map[string]instagramLogin{}, sessions: map[string]instagramLogin{}}
}

func (s *instagramLoginStore) gc(now time.Time) {
	for k, v := range s.states {
		if now.After(v.expiresAt) {
			delete(s.states, k)
		}
	}
	for k, v := range s.sessions {
		if now.After(v.expiresAt) {
			delete(s.sessions, k)
		}
	}
}

func (s *instagramLoginStore) newState(userID int) (string, error) {
	key, err := randomKey()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.gc(now)
	s.states[key] = instagramLogin{userID: userID, expiresAt: now.Add(instagramLoginTTL)}
	return key, nil
}

// takeState — state одноразовый и принадлежит тому, кто начал вход.
func (s *instagramLoginStore) takeState(key string, userID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gc(time.Now())
	v, ok := s.states[key]
	if !ok || v.userID != userID {
		return false
	}
	delete(s.states, key)
	return true
}

func (s *instagramLoginStore) newSession(userID int, token string, accounts []meta.InstagramAccount) (string, error) {
	key, err := randomKey()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.gc(now)
	s.sessions[key] = instagramLogin{userID: userID, token: token, accounts: accounts, expiresAt: now.Add(instagramLoginTTL)}
	return key, nil
}

func (s *instagramLoginStore) session(key string, userID int) (instagramLogin, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gc(time.Now())
	v, ok := s.sessions[key]
	if !ok || v.userID != userID {
		return instagramLogin{}, false
	}
	return v, true
}

func (s *instagramLoginStore) dropSession(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, key)
}

func randomKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SetMeta подключает вход через Facebook (приложение Meta компании).
func (h *WazzupHandler) SetMeta(client *meta.Client) {
	h.meta = client
	h.igLogins = newInstagramLoginStore()
}

// instagramPrecheck — права (админ/руководство) и настроенное приложение Meta.
func (h *WazzupHandler) instagramPrecheck(c *gin.Context) (int, bool) {
	userID, roleID := getUserAndRole(c)
	if roleID != authz.RoleSystemAdmin && roleID != authz.RoleManagement {
		forbidden(c, "Forbidden")
		return 0, false
	}
	if h.meta == nil || !h.meta.Configured() || h.igLogins == nil {
		badRequestWithCode(c, "meta_app_not_configured",
			"Вход через Facebook не настроен на сервере: нужны META_APP_ID, META_APP_SECRET и META_REDIRECT_URL")
		return 0, false
	}
	return userID, true
}

// POST /integrations/wazzup/instagram/login — ссылка входа через Facebook.
func (h *WazzupHandler) InstagramLogin(c *gin.Context) {
	userID, ok := h.instagramPrecheck(c)
	if !ok {
		return
	}
	state, err := h.igLogins.newState(userID)
	if err != nil {
		internalError(c, "failed to start facebook login")
		return
	}
	authURL, err := h.meta.DialogURL(state)
	if err != nil {
		internalError(c, "failed to start facebook login")
		return
	}
	c.JSON(http.StatusOK, gin.H{"auth_url": authURL})
}

type instagramAccountsRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// POST /integrations/wazzup/instagram/accounts — обмен code на токен и список
// аккаунтов Instagram, доступных вошедшему.
func (h *WazzupHandler) InstagramAccounts(c *gin.Context) {
	userID, ok := h.instagramPrecheck(c)
	if !ok {
		return
	}
	var req instagramAccountsRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		badRequest(c, "Нет кода входа Facebook")
		return
	}
	if !h.igLogins.takeState(strings.TrimSpace(req.State), userID) {
		badRequestWithCode(c, "facebook_login_expired", "Вход через Facebook устарел или начат другим пользователем — начните подключение заново")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	token, err := h.meta.ExchangeCode(ctx, req.Code)
	if err != nil {
		log.Printf("integration=meta operation=exchange_code status=failed user=%d err=%v", userID, err)
		writeFacebookError(c, err, "Facebook не подтвердил вход")
		return
	}
	accounts, err := h.meta.InstagramAccounts(ctx, token)
	if err != nil {
		log.Printf("integration=meta operation=instagram_accounts status=failed user=%d err=%v", userID, err)
		writeFacebookError(c, err, "Не удалось получить страницы Facebook")
		return
	}
	session, err := h.igLogins.newSession(userID, token, accounts)
	if err != nil {
		internalError(c, "failed to keep facebook login")
		return
	}
	log.Printf("integration=meta operation=instagram_accounts status=ok user=%d accounts=%d", userID, len(accounts))
	c.JSON(http.StatusOK, gin.H{"session": session, "accounts": accounts})
}

type instagramConnectRequest struct {
	Session string `json:"session"`
	InstID  string `json:"inst_id"`
}

// POST /integrations/wazzup/channels/instagram — создать канал Instagram в
// дочернем аккаунте Wazzup по выбранному после входа аккаунту.
func (h *WazzupHandler) ConnectInstagram(c *gin.Context) {
	userID, ok := h.instagramPrecheck(c)
	if !ok {
		return
	}
	var req instagramConnectRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.InstID) == "" {
		badRequest(c, "Выберите аккаунт Instagram")
		return
	}
	login, found := h.igLogins.session(strings.TrimSpace(req.Session), userID)
	if !found {
		badRequestWithCode(c, "facebook_login_expired", "Вход через Facebook устарел — начните подключение заново")
		return
	}
	var picked *meta.InstagramAccount
	for i := range login.accounts {
		if login.accounts[i].InstID == strings.TrimSpace(req.InstID) {
			picked = &login.accounts[i]
			break
		}
	}
	if picked == nil {
		badRequest(c, "Этого аккаунта Instagram нет среди доступных после входа")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	res, err := h.svc.ConnectInstagram(ctx, wz.InstagramCredentials{
		Username:  picked.Username,
		PageID:    picked.PageID,
		InstID:    picked.InstID,
		UserToken: login.token,
	})
	if err != nil {
		var createErr *wz.ChannelCreateError
		if errors.As(err, &createErr) {
			c.JSON(http.StatusBadGateway, gin.H{
				"error_code": "wazzup_channel_create_failed",
				"message":    "Wazzup не создал канал Instagram: " + createErr.Detail,
			})
			return
		}
		writeWazzupError(c, err, "failed to connect instagram")
		return
	}
	h.igLogins.dropSession(req.Session)
	c.JSON(http.StatusOK, res)
}

func writeFacebookError(c *gin.Context, err error, prefix string) {
	var ge *meta.GraphError
	if errors.As(err, &ge) {
		c.JSON(http.StatusBadGateway, gin.H{"error_code": "facebook_error", "message": prefix + ": " + ge.Message})
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		c.JSON(http.StatusGatewayTimeout, gin.H{"error_code": "facebook_timeout", "message": prefix + ": Facebook не ответил вовремя"})
		return
	}
	c.JSON(http.StatusBadGateway, gin.H{"error_code": "facebook_error", "message": prefix})
}
