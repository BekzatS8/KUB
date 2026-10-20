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
	"turcompany/internal/repositories"
)

// Подключение Instagram к VISARIO (дочерний аккаунт Wazzup):
//
//  0. GET/PUT /integrations/wazzup/instagram/settings — ключи приложения Meta
//     вводит администратор в CRM (один раз на CRM, проверяются в Facebook);
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

// metaSource собирает клиент Meta на каждый запрос: ключи, введённые
// администратором в CRM, важнее ключей из переменных сервера (запасной путь).
type metaSource struct {
	repo    repositories.MetaSettingsRepository // nil — только ключи сервера
	base    meta.Config
	timeout time.Duration
}

// client — клиент Meta и откуда взяты ключи: "crm", "server" или "".
func (m *metaSource) client(ctx context.Context) (*meta.Client, string, error) {
	cfg := m.base
	source := ""
	if cfg.AppID != "" && cfg.AppSecret != "" {
		source = "server"
	}
	if m.repo != nil {
		saved, err := m.repo.Get(ctx)
		if err != nil {
			return nil, "", err
		}
		if saved != nil && strings.TrimSpace(saved.AppID) != "" && strings.TrimSpace(saved.AppSecret) != "" {
			cfg.AppID, cfg.AppSecret = saved.AppID, saved.AppSecret
			source = "crm"
		}
	}
	return meta.NewClient(cfg, m.timeout), source, nil
}

// SetMeta подключает вход через Facebook. base — адрес возврата, версия Graph
// API, права и (необязательно) ключи из переменных сервера.
func (h *WazzupHandler) SetMeta(repo repositories.MetaSettingsRepository, base meta.Config, timeout time.Duration) {
	h.metaSrc = &metaSource{repo: repo, base: base, timeout: timeout}
	h.igLogins = newInstagramLoginStore()
}

func instagramAdmin(c *gin.Context) (int, bool) {
	userID, roleID := getUserAndRole(c)
	if roleID != authz.RoleSystemAdmin && roleID != authz.RoleManagement {
		forbidden(c, "Forbidden")
		return 0, false
	}
	return userID, true
}

// instagramPrecheck — права (админ/руководство) и настроенное приложение Meta.
func (h *WazzupHandler) instagramPrecheck(c *gin.Context) (int, *meta.Client, bool) {
	userID, ok := instagramAdmin(c)
	if !ok {
		return 0, nil, false
	}
	if h.metaSrc == nil || h.igLogins == nil {
		badRequestWithCode(c, "meta_app_not_configured", "Вход через Facebook недоступен на этом сервере")
		return 0, nil, false
	}
	client, _, err := h.metaSrc.client(c.Request.Context())
	if err != nil {
		internalError(c, "failed to load meta app settings")
		return 0, nil, false
	}
	if !client.Configured() {
		badRequestWithCode(c, "meta_app_not_configured",
			"Приложение Meta не настроено — введите App ID и App Secret в окне «Подключить Instagram»")
		return 0, nil, false
	}
	return userID, client, true
}

type metaSettingsView struct {
	Configured bool   `json:"configured"`
	AppID      string `json:"app_id"`
	// HasSecret — секрет сохранён в CRM (пустое поле при правке его оставит).
	HasSecret   bool   `json:"has_secret"`
	RedirectURL string `json:"redirect_url"`
	// Source — откуда ключи: crm (введены в CRM) или server (переменные).
	Source string `json:"source,omitempty"`
}

func (h *WazzupHandler) metaSettingsView(c *gin.Context) (*metaSettingsView, bool) {
	client, source, err := h.metaSrc.client(c.Request.Context())
	if err != nil {
		internalError(c, "failed to load meta app settings")
		return nil, false
	}
	return &metaSettingsView{
		Configured:  client.Configured(),
		AppID:       client.AppID(),
		HasSecret:   source == "crm",
		RedirectURL: client.RedirectURL(),
		Source:      source,
	}, true
}

// GET /integrations/wazzup/instagram/settings — ключи приложения Meta (без
// секрета) и адрес возврата, который нужно указать в приложении.
func (h *WazzupHandler) InstagramSettings(c *gin.Context) {
	if _, ok := instagramAdmin(c); !ok {
		return
	}
	if h.metaSrc == nil {
		badRequestWithCode(c, "meta_app_not_configured", "Вход через Facebook недоступен на этом сервере")
		return
	}
	view, ok := h.metaSettingsView(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, view)
}

type metaSettingsRequest struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

// PUT /integrations/wazzup/instagram/settings — сохранить ключи приложения
// Meta. Ключи сначала проверяются в Facebook; пустой секрет — оставить прежний.
func (h *WazzupHandler) SaveInstagramSettings(c *gin.Context) {
	userID, ok := instagramAdmin(c)
	if !ok {
		return
	}
	if h.metaSrc == nil || h.metaSrc.repo == nil {
		badRequestWithCode(c, "meta_app_not_configured", "Вход через Facebook недоступен на этом сервере")
		return
	}
	var req metaSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	appID := strings.TrimSpace(req.AppID)
	secret := strings.TrimSpace(req.AppSecret)
	if appID == "" || strings.Trim(appID, "0123456789") != "" {
		badRequest(c, "App ID — это число из настроек приложения Meta")
		return
	}
	if secret == "" {
		saved, err := h.metaSrc.repo.Get(c.Request.Context())
		if err != nil {
			internalError(c, "failed to load meta app settings")
			return
		}
		if saved == nil || saved.AppSecret == "" {
			badRequest(c, "Укажите App Secret")
			return
		}
		secret = saved.AppSecret
	}

	cfg := h.metaSrc.base
	cfg.AppID, cfg.AppSecret = appID, secret
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	if err := meta.NewClient(cfg, h.metaSrc.timeout).VerifyApp(ctx); err != nil {
		log.Printf("integration=meta operation=verify_app status=failed user=%d app=%s err=%v", userID, appID, err)
		var ge *meta.GraphError
		if errors.As(err, &ge) {
			badRequestWithCode(c, "meta_app_invalid", "Facebook не принял ключи: "+ge.Message)
			return
		}
		writeFacebookError(c, err, "Не удалось проверить ключи в Facebook")
		return
	}
	if err := h.metaSrc.repo.Save(c.Request.Context(), appID, strings.TrimSpace(req.AppSecret), userID); err != nil {
		internalError(c, "failed to save meta app settings")
		return
	}
	log.Printf("integration=meta operation=save_app status=ok user=%d app=%s", userID, appID)
	view, ok := h.metaSettingsView(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, view)
}

// POST /integrations/wazzup/instagram/login — ссылка входа через Facebook.
func (h *WazzupHandler) InstagramLogin(c *gin.Context) {
	userID, client, ok := h.instagramPrecheck(c)
	if !ok {
		return
	}
	state, err := h.igLogins.newState(userID)
	if err != nil {
		internalError(c, "failed to start facebook login")
		return
	}
	authURL, err := client.DialogURL(state)
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
	userID, client, ok := h.instagramPrecheck(c)
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

	token, err := client.ExchangeCode(ctx, req.Code)
	if err != nil {
		log.Printf("integration=meta operation=exchange_code status=failed user=%d err=%v", userID, err)
		writeFacebookError(c, err, "Facebook не подтвердил вход")
		return
	}
	accounts, err := client.InstagramAccounts(ctx, token)
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
	userID, _, ok := h.instagramPrecheck(c)
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
				"message":    "Wazzup не создал канал Instagram: " + createErr.Detail + requestedAtSuffix(createErr.RequestedAtMSK),
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

func requestedAtSuffix(at string) string {
	if at == "" {
		return ""
	}
	return " (запрос " + at + ")"
}
