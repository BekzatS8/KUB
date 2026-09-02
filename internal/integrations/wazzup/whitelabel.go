package wazzup

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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

// WhiteLabelClient получает client_access_token дочернего аккаунта и строит
// ссылку на iframe подключения каналов. Токен кэшируется в памяти (живёт сутки),
// при истечении переполучается полным флоу (machine_token → token-exchange).
type WhiteLabelClient struct {
	cfg  WhiteLabelConfig
	http *http.Client

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
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

	machineToken, err := c.fetchMachineToken(ctx)
	if err != nil {
		return "", err
	}
	access, expiresIn, err := c.exchangeToken(ctx, machineToken)
	if err != nil {
		return "", err
	}
	c.accessToken = access
	if expiresIn <= 0 {
		expiresIn = 86400 // сутки по умолчанию
	}
	c.expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	return access, nil
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

// exchangeToken — шаг 3 доки: token-exchange → client_access_token дочки.
func (c *WhiteLabelClient) exchangeToken(ctx context.Context, machineToken string) (string, int, error) {
	payload := map[string]any{
		"grant_type": "urn:ietf:params:oauth:grant-type:token-exchange",
		"token_exchange_data": map[string]any{
			"subject_token":      machineToken,
			"subject_token_type": "urn:wazzup:oauth:token-type:machine_token",
			"requested_subject":  c.cfg.AccountID,
			"scope":              c.cfg.Scope,
		},
	}
	body, err := c.doAuthed(ctx, http.MethodPost, "/v2/oauth/token", c.basicAuth(), payload)
	if err != nil {
		return "", 0, fmt.Errorf("token exchange: %w", err)
	}
	var resp struct {
		Data struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", 0, fmt.Errorf("decode token exchange: %w", err)
	}
	if strings.TrimSpace(resp.Data.AccessToken) == "" {
		return "", 0, fmt.Errorf("empty client access token")
	}
	return resp.Data.AccessToken, resp.Data.ExpiresIn, nil
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
