package wazzup

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// WhiteLabelConfig — партнёрские (tech-partner) доступы Wazzup для встроенного
// добавления каналов. Все поля — секреты, живут только на бэкенде.
type WhiteLabelConfig struct {
	BaseURL   string // https://tech.wazzup24.com
	Email     string // логин кабинета партнёра (Basic auth)
	Password  string // пароль кабинета партнёра (Basic auth)
	ClientID  string // partner_client_id (для refresh)
	AccountID string // account_id дочернего аккаунта клиента (requested_subject)
	Scope     string // напр. "transport,crm"
}

const defaultWLBaseURL = "https://tech.wazzup24.com"

// WhiteLabelClient получает client_access_token дочернего аккаунта, строит
// ссылку на iframe подключения каналов и служит поставщиком токенов для
// PartnerClient. Токен живёт час и кэшируется в памяти: при истечении
// обновляется по refresh_token (живёт 7 суток), а если тот протух — полным
// флоу (machine_token → token-exchange).
type WhiteLabelClient struct {
	cfg  WhiteLabelConfig
	http *http.Client

	mu           sync.Mutex
	accessToken  string
	refreshToken string
	expiresAt    time.Time
}

// AccessToken отдаёт актуальный client_access_token дочернего аккаунта.
// Реализует TokenProvider для PartnerClient.
func (c *WhiteLabelClient) AccessToken(ctx context.Context) (string, error) {
	if !c.Configured() {
		return "", fmt.Errorf("white label is not configured")
	}
	return c.clientAccessToken(ctx)
}

// InvalidateToken сбрасывает кэш токена. Нужен, когда провайдер ответил 401
// раньше расчётного истечения (отозвали токен, сменили доступы).
func (c *WhiteLabelClient) InvalidateToken() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accessToken = ""
	c.expiresAt = time.Time{}
}

func NewWhiteLabelClient(cfg WhiteLabelConfig, timeout time.Duration) *WhiteLabelClient {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = defaultWLBaseURL
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if strings.TrimSpace(cfg.Scope) == "" {
		cfg.Scope = "transport,crm"
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &WhiteLabelClient{cfg: cfg, http: &http.Client{Timeout: timeout}}
}

// Configured сообщает, заданы ли партнёрские доступы (иначе White Label выключен).
func (c *WhiteLabelClient) Configured() bool {
	return c != nil &&
		strings.TrimSpace(c.cfg.Email) != "" &&
		strings.TrimSpace(c.cfg.Password) != "" &&
		strings.TrimSpace(c.cfg.AccountID) != ""
}

func (c *WhiteLabelClient) basicAuth() string {
	raw := c.cfg.Email + ":" + c.cfg.Password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}

// ChannelsIframeLink возвращает ссылку на iframe подключения канала указанного
// транспорта (whatsapp/wapi/tgapi/maxbot/max/vk/cian). Её фронт вставляет в <iframe>.
func (c *WhiteLabelClient) ChannelsIframeLink(ctx context.Context, transport string) (string, error) {
	if !c.Configured() {
		return "", fmt.Errorf("white label is not configured")
	}
	token, err := c.clientAccessToken(ctx)
	if err != nil {
		return "", err
	}
	options := map[string]any{}
	if t := strings.TrimSpace(transport); t != "" {
		// Конкретный транспорт — iframe откроется сразу на его подключении.
		// Без транспорта — общий экран выбора канала (как в кабинете Wazzup).
		options["transport"] = t
	}
	payload := map[string]any{"options": options}
	body, err := c.doAuthed(ctx, http.MethodPost, "/v2/iframe-links/channels", "Bearer "+token, payload)
	if err != nil {
		return "", err
	}
	var resp struct {
		Data struct {
			Link string `json:"link"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode iframe-links response: %w", err)
	}
	if strings.TrimSpace(resp.Data.Link) == "" {
		return "", fmt.Errorf("empty iframe link in response")
	}
	return resp.Data.Link, nil
}

// clientAccessToken возвращает кэшированный токен дочки или получает новый.
func (c *WhiteLabelClient) clientAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Небольшой запас (60с) до истечения, чтобы не отдать почти протухший токен.
	if c.accessToken != "" && time.Now().Add(time.Minute).Before(c.expiresAt) {
		return c.accessToken, nil
	}

	// Токен живёт час, refresh — неделю. Пока refresh жив, обновляемся им:
	// это один запрос вместо двух и не дёргает Basic-авторизацию партнёра.
	if c.refreshToken != "" && strings.TrimSpace(c.cfg.ClientID) != "" {
		if access, refresh, expiresIn, err := c.refreshAccessToken(ctx); err == nil {
			c.storeTokens(access, refresh, expiresIn)
			return access, nil
		} else {
			// refresh протух (7 дней) или отозван — идём полным флоу.
			log.Printf("integration=wazzup operation=wl_refresh status=failed err=%v", err)
			c.refreshToken = ""
		}
	}

	machineToken, err := c.fetchMachineToken(ctx)
	if err != nil {
		return "", err
	}
	access, refresh, expiresIn, err := c.exchangeToken(ctx, machineToken)
	if err != nil {
		return "", err
	}
	c.storeTokens(access, refresh, expiresIn)
	return access, nil
}

// storeTokens кладёт свежую пару токенов в кэш. Вызывается под c.mu.
func (c *WhiteLabelClient) storeTokens(access, refresh string, expiresIn int) {
	c.accessToken = access
	if strings.TrimSpace(refresh) != "" {
		c.refreshToken = refresh
	}
	if expiresIn <= 0 {
		expiresIn = 3600 // час — документированное время жизни client_access_token
	}
	c.expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
}

// refreshAccessToken обновляет токен по refresh_token (grant_type=refresh_token).
func (c *WhiteLabelClient) refreshAccessToken(ctx context.Context) (access, refresh string, expiresIn int, err error) {
	payload := map[string]any{
		"grant_type": "refresh_token",
		"refresh_token_data": map[string]any{
			"refresh_token": c.refreshToken,
			"client_id":     strings.TrimSpace(c.cfg.ClientID),
		},
	}
	body, err := c.doAuthed(ctx, http.MethodPost, "/v2/oauth/token", c.basicAuth(), payload)
	if err != nil {
		return "", "", 0, err
	}
	var resp struct {
		Data struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int    `json:"expires_in"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", 0, fmt.Errorf("decode refresh token: %w", err)
	}
	if strings.TrimSpace(resp.Data.AccessToken) == "" {
		return "", "", 0, fmt.Errorf("empty access token in refresh response")
	}
	return resp.Data.AccessToken, resp.Data.RefreshToken, resp.Data.ExpiresIn, nil
}

// fetchMachineToken — шаг 2 доки: client_credentials → machine_token.
func (c *WhiteLabelClient) fetchMachineToken(ctx context.Context) (string, error) {
	payload := map[string]any{
		"grant_type": "client_credentials",
		"client_credentials_data": map[string]any{
			"scope": c.cfg.Scope,
		},
	}
	body, err := c.doAuthed(ctx, http.MethodPost, "/v2/oauth/token", c.basicAuth(), payload)
	if err != nil {
		return "", fmt.Errorf("machine token: %w", err)
	}
	var resp struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode machine token: %w", err)
	}
	if strings.TrimSpace(resp.Data.AccessToken) == "" {
		return "", fmt.Errorf("empty machine token")
	}
	return resp.Data.AccessToken, nil
}

// exchangeToken — шаг 3 доки: token-exchange → client_access_token дочки
// плюс refresh_token для последующего обновления.
func (c *WhiteLabelClient) exchangeToken(ctx context.Context, machineToken string) (string, string, int, error) {
	// Wazzup требует requested_subject как «number string» — только цифры.
	// В кабинете account_id может отображаться с дефисом (7204-2419) — чистим.
	requestedSubject := digitsOnly(c.cfg.AccountID)
	payload := map[string]any{
		"grant_type": "urn:ietf:params:oauth:grant-type:token-exchange",
		"token_exchange_data": map[string]any{
			"subject_token":      machineToken,
			"subject_token_type": "urn:wazzup:oauth:token-type:machine_token",
			"requested_subject":  requestedSubject,
			"scope":              c.cfg.Scope,
		},
	}
	body, err := c.doAuthed(ctx, http.MethodPost, "/v2/oauth/token", c.basicAuth(), payload)
	if err != nil {
		return "", "", 0, fmt.Errorf("token exchange: %w", err)
	}
	var resp struct {
		Data struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int    `json:"expires_in"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", 0, fmt.Errorf("decode token exchange: %w", err)
	}
	if strings.TrimSpace(resp.Data.AccessToken) == "" {
		return "", "", 0, fmt.Errorf("empty client access token")
	}
	return resp.Data.AccessToken, resp.Data.RefreshToken, resp.Data.ExpiresIn, nil
}

// digitsOnly оставляет в строке только цифры (для account_id/requested_subject).
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (c *WhiteLabelClient) doAuthed(ctx context.Context, method, path, authHeader string, payload any) ([]byte, error) {
	var reader io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", authHeader)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wazzup wl request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("wazzup wl %s %s: status=%d body=%s", method, path, resp.StatusCode, string(body))
	}
	return body, nil
}

// BaseURL отдаёт базовый адрес партнёрского API (для логов и диагностики).
func (c *WhiteLabelClient) BaseURL() string {
	if c == nil {
		return ""
	}
	return c.cfg.BaseURL
}
