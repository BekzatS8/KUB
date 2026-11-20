package wazzup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.wazzup24.com"

type Client interface {
	PatchWebhooks(ctx context.Context, apiKey, webhooksURI, crmKey string) error
	UpsertUsers(ctx context.Context, apiKey string, users []UserUpsert) error
	CreateIframe(ctx context.Context, apiKey string, req CreateIframeRequest) (string, error)
	ListChannels(ctx context.Context, apiKey string) ([]Channel, error)
	SendMessage(ctx context.Context, apiKey string, req SendMessageRequest) (*SendMessageResponse, error)
	// DeleteChannel удаляет канал у провайдера. Возвращает
	// ErrChannelDeleteUnsupported, если драйвер этого не умеет — тогда
	// вызывающий код ограничивается удалением строки в CRM.
	DeleteChannel(ctx context.Context, apiKey, externalChannelID string, deleteChats bool) error
	// SyncUserRoles выдаёт сотрудникам роли на каналах. Без роли сотрудник не
	// видит чатов в окне мессенджера вообще. Возвращает
	// ErrUserRolesUnsupported, если драйвер этого не умеет.
	SyncUserRoles(ctx context.Context, apiKey string, roles []UserChannelRole) error
}

// UserChannelRole — роль сотрудника на конкретном канале.
type UserChannelRole struct {
	ChannelID string
	UserID    string
	// Role — seller (свои чаты), manager (все чаты + отправка),
	// auditor (все чаты, без отправки).
	Role string
	// AllowGetNewClients — получает ли сотрудник обращения от новых контактов.
	AllowGetNewClients bool
}

type UserUpsert struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CreateIframeRequest struct {
	User       UserUpsert        `json:"user"`
	Scope      string            `json:"scope"`
	Filter     []IframeChat      `json:"filter,omitempty"`
	ActiveChat *IframeActiveChat `json:"activeChat,omitempty"`
	Options    map[string]any    `json:"options,omitempty"`
}

type IframeChat struct {
	ChatType string `json:"chatType,omitempty"`
	ChatID   string `json:"chatId,omitempty"`
	Name     string `json:"name,omitempty"`
}

type IframeActiveChat struct {
	ChannelID string `json:"channelId,omitempty"`
	ChatType  string `json:"chatType,omitempty"`
	ChatID    string `json:"chatId,omitempty"`
}

type SendMessageRequest struct {
	ChannelID string `json:"channelId,omitempty"`
	// ChatType — тип транспорта (whatsapp/telegram/instagram). Wazzup v3
	// требует его для отправки, иначе не может создать диалог (нельзя написать
	// первым). Раньше поле отсутствовало → «не удаётся написать клиенту».
	ChatType string `json:"chatType,omitempty"`
	ChatID   string `json:"chatId"`
	Text     string `json:"text"`
	// CRMUserID — автор сообщения из списка пользователей CRM (тот же id, что
	// отдаёт /integrations/wazzup/crm/:token/users). Без него Wazzup считает
	// автором саму интеграцию и подписывает сообщение «API • Admin», хотя писал
	// менеджер (обратная связь заказчика 18.09.2026).
	CRMUserID string `json:"crmUserId,omitempty"`
	// ContentURI — файл по прямой ссылке: провайдер скачивает его сам и
	// отправляет вложением. Вместо текста, а не вместе с ним.
	ContentURI string `json:"contentUri,omitempty"`
	// Имя, тип и размер файла — для партнёрского API (attachment); v3 берёт
	// их из ответа по ссылке.
	FileName string `json:"-"`
	FileMime string `json:"-"`
	FileSize int64  `json:"-"`
}

type SendMessageResponse struct {
	MessageID string
}

type Channel struct {
	ID         string         `json:"id"`
	Transport  string         `json:"transport"`
	Name       string         `json:"name"`
	Username   string         `json:"username"`
	Phone      string         `json:"phone"`
	Status     string         `json:"status"`
	RawPayload map[string]any `json:"raw_payload,omitempty"`
}

type HTTPClient struct {
	baseURL string
	http    *http.Client
	retries int
	retryIn time.Duration
}

func NewHTTPClient(baseURL string, timeout time.Duration, retries int, retryDelay time.Duration) *HTTPClient {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if retries < 0 {
		retries = 0
	}
	if retryDelay <= 0 {
		retryDelay = 300 * time.Millisecond
	}
	return &HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: timeout},
		retries: retries,
		retryIn: retryDelay,
	}
}

func (c *HTTPClient) PatchWebhooks(ctx context.Context, apiKey, webhooksURI, crmKey string) error {
	payload := map[string]any{
		"webhooksUri": webhooksURI,
		"crmKey":      crmKey,
		"subscriptions": map[string]any{
			"messagesAndStatuses": true,
			"channelsUpdates":     true,
		},
	}
	_, err := c.doJSON(ctx, http.MethodPatch, "/v3/webhooks", apiKey, payload)
	return err
}

func (c *HTTPClient) UpsertUsers(ctx context.Context, apiKey string, users []UserUpsert) error {
	_, err := c.doJSON(ctx, http.MethodPost, "/v3/users", apiKey, users)
	return err
}

func (c *HTTPClient) CreateIframe(ctx context.Context, apiKey string, req CreateIframeRequest) (string, error) {
	payload := req
	if strings.TrimSpace(payload.Scope) == "" {
		payload.Scope = "global"
	}
	body, err := c.doJSON(ctx, http.MethodPost, "/v3/iframe", apiKey, payload)
	if err != nil {
		return "", err
	}
	var resp struct {
		IframeURL string `json:"iframeUrl"`
		URL       string `json:"url"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode iframe response: %w", err)
	}
	if strings.TrimSpace(resp.IframeURL) != "" {
		return strings.TrimSpace(resp.IframeURL), nil
	}
	if strings.TrimSpace(resp.URL) != "" {
		return strings.TrimSpace(resp.URL), nil
	}
	return "", fmt.Errorf("iframe url is empty")
}

func (c *HTTPClient) ListChannels(ctx context.Context, apiKey string) ([]Channel, error) {
	body, err := c.doJSON(ctx, http.MethodGet, "/v3/channels", apiKey, nil)
	if err != nil {
		return nil, err
	}
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("decode channels response: %w", err)
	}

	rawItems := extractChannelItems(root)
	channels := make([]Channel, 0, len(rawItems))
	for _, item := range rawItems {
		ch := mapChannel(item)
		if strings.TrimSpace(ch.ID) == "" {
			continue
		}
		channels = append(channels, ch)
	}
	return channels, nil
}

func (c *HTTPClient) SendMessage(ctx context.Context, apiKey string, req SendMessageRequest) (*SendMessageResponse, error) {
	req.ChatID = strings.TrimSpace(req.ChatID)
	req.Text = strings.TrimSpace(req.Text)
	req.ContentURI = strings.TrimSpace(req.ContentURI)
	if req.ChatID == "" || (req.Text == "" && req.ContentURI == "") {
		return nil, fmt.Errorf("chatId and text or contentUri are required")
	}
	if req.ContentURI != "" {
		// Файл отправляется отдельным сообщением без текста.
		req.Text = ""
	}
	body, err := c.doJSON(ctx, http.MethodPost, "/v3/message", apiKey, req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		MessageID string `json:"messageId"`
		ID        string `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode send message response: %w", err)
	}
	messageID := strings.TrimSpace(resp.MessageID)
	if messageID == "" {
		messageID = strings.TrimSpace(resp.ID)
	}
	return &SendMessageResponse{MessageID: messageID}, nil
}

func (c *HTTPClient) doJSON(ctx context.Context, method, path, apiKey string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if payload != nil {
			if seeker, ok := body.(io.Seeker); ok {
				_, _ = seeker.Seek(0, io.SeekStart)
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
		if err != nil {
			return nil, fmt.Errorf("new request: %w", err)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("wazzup request: %w", err)
		} else {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return body, nil
			}
			traceID := strings.TrimSpace(resp.Header.Get("trace-id"))
			if traceID != "" {
				lastErr = fmt.Errorf("wazzup %s %s failed: status=%d trace_id=%s body=%s", method, path, resp.StatusCode, traceID, string(body))
			} else {
				lastErr = fmt.Errorf("wazzup %s %s failed: status=%d body=%s", method, path, resp.StatusCode, string(body))
			}
			if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
				return nil, lastErr
			}
		}
		if attempt < c.retries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.retryIn):
			}
		}
	}
	if lastErr == nil {
		lastErr = errors.New("wazzup request failed")
	}
	return nil, lastErr
}

func extractChannelItems(root any) []map[string]any {
	switch v := root.(type) {
	case []any:
		return anySliceToMaps(v)
	case map[string]any:
		for _, key := range []string{"channels", "data", "items", "result"} {
			if arr, ok := v[key].([]any); ok {
				return anySliceToMaps(arr)
			}
		}
	}
	return nil
}

func anySliceToMaps(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func mapChannel(item map[string]any) Channel {
	ch := Channel{
		ID:         firstString(item, "id", "channelId", "channel_id", "guid"),
		Transport:  normalizeTransport(firstString(item, "transport", "type", "channelType", "channel_type")),
		Name:       firstString(item, "name", "title", "displayName", "display_name"),
		Username:   firstString(item, "username", "login", "accountName", "account_name"),
		Phone:      firstString(item, "phone", "phoneNumber", "phone_number"),
		Status:     strings.ToLower(strings.TrimSpace(firstString(item, "status", "state"))),
		RawPayload: item,
	}
	if ch.Transport == "" {
		ch.Transport = "whatsapp"
	}
	if ch.Status == "" {
		ch.Status = "unknown"
	}
	if ch.Name == "" {
		ch.Name = ch.Username
	}
	if ch.Name == "" {
		ch.Name = ch.Phone
	}
	if ch.Name == "" {
		ch.Name = ch.ID
	}
	return ch
}

func firstString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := item[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case float64:
			return fmt.Sprintf("%.0f", v)
		case int:
			return fmt.Sprintf("%d", v)
		}
	}
	return ""
}

func normalizeTransport(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "wa", "waba", "whatsapp":
		return "whatsapp"
	case "tg", "telegram":
		return "telegram"
	case "ig", "instagram", "instagram_direct", "direct", "instapi":
		return "instagram"
	default:
		return v
	}
}

// DeleteChannel в User API v3 не поддерживается: провайдер отдаёт только
// GET /v3/channels, а отключают канал вручную в кабинете Wazzup.
// UpsertContacts создаёт или обновляет контакты (POST /v3/contacts). Ссылка
// uri — кнопка в выпадающем списке «Сделки» в шапке чата Wazzup.
func (c *HTTPClient) UpsertContacts(ctx context.Context, apiKey string, contacts []ContactUpsert) error {
	items := make([]map[string]any, 0, len(contacts))
	for _, ct := range contacts {
		item := map[string]any{
			"id":                ct.ID,
			"responsibleUserId": ct.ResponsibleUserID,
			"name":              ct.Name,
			"contactData": []map[string]any{{
				"chatType": normalizePartnerChatType(ct.ChatType),
				"chatId":   ct.ChatID,
			}},
		}
		if ct.URI != "" {
			item["uri"] = ct.URI
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil
	}
	_, err := c.doJSON(ctx, http.MethodPost, "/v3/contacts", apiKey, items)
	return err
}

// getLarge — GET без ограничения тела в 2 КБ (списки контактов). Статус 404
// возвращается без ошибки: «нет такого».
func (c *HTTPClient) getLarge(ctx context.Context, path, apiKey string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("wazzup request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode == http.StatusNotFound {
		return nil, resp.StatusCode, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(body) > 512 {
			body = body[:512]
		}
		return nil, resp.StatusCode, fmt.Errorf("wazzup GET %s failed: status=%d body=%s", path, resp.StatusCode, string(body))
	}
	return body, resp.StatusCode, nil
}

// realResponsible — ответственный контакта, если он назначен («0» — никого).
func realResponsible(id string) string {
	id = strings.TrimSpace(id)
	if id == "0" {
		return ""
	}
	return id
}

// ContactResponsibles — ответственные всех контактов аккаунта (GET
// /v3/contacts постранично по 100): id контакта → id сотрудника Wazzup.
func (c *HTTPClient) ContactResponsibles(ctx context.Context, apiKey string) (map[string]string, error) {
	out := map[string]string{}
	for offset := 0; offset < 100000; offset += 100 {
		body, _, err := c.getLarge(ctx, fmt.Sprintf("/v3/contacts?offset=%d", offset), apiKey)
		if err != nil {
			return nil, err
		}
		var page struct {
			Count int `json:"count"`
			Data  []struct {
				ID                string `json:"id"`
				ResponsibleUserID string `json:"responsibleUserId"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decode contacts: %w", err)
		}
		for _, ct := range page.Data {
			out[ct.ID] = realResponsible(ct.ResponsibleUserID)
		}
		if len(page.Data) < 100 || offset+100 >= page.Count {
			break
		}
	}
	return out, nil
}

// ContactResponsible — ответственный одного контакта; "" — контакта нет или
// за ним никто не закреплён.
func (c *HTTPClient) ContactResponsible(ctx context.Context, apiKey, contactID string) (string, error) {
	body, status, err := c.getLarge(ctx, "/v3/contacts/"+url.PathEscape(contactID), apiKey)
	if err != nil || status == http.StatusNotFound {
		return "", err
	}
	var ct struct {
		ResponsibleUserID string `json:"responsibleUserId"`
	}
	if err := json.Unmarshal(body, &ct); err != nil {
		return "", fmt.Errorf("decode contact: %w", err)
	}
	return realResponsible(ct.ResponsibleUserID), nil
}

// UpsertDeals создаёт или обновляет сделки (POST /v3/deals): лид CRM в
// выпадающем списке «Сделки» в шапке чата Wazzup со ссылкой на его карточку.
func (c *HTTPClient) UpsertDeals(ctx context.Context, apiKey string, deals []DealUpsert) error {
	items := make([]map[string]any, 0, len(deals))
	for _, d := range deals {
		items = append(items, map[string]any{
			"id":                d.ID,
			"responsibleUserId": d.ResponsibleUserID,
			"name":              d.Name,
			"uri":               d.URI,
			"contacts":          d.ContactIDs,
			"closed":            d.Closed,
		})
	}
	if len(items) == 0 {
		return nil
	}
	_, err := c.doJSON(ctx, http.MethodPost, "/v3/deals", apiKey, items)
	return err
}

func (c *HTTPClient) DeleteChannel(_ context.Context, _, _ string, _ bool) error {
	return ErrChannelDeleteUnsupported
}

// SyncUserRoles в User API v3 не поддерживается: роли выдаются вручную в
// кабинете Wazzup, отдельного метода у провайдера нет.
func (c *HTTPClient) SyncUserRoles(_ context.Context, _ string, _ []UserChannelRole) error {
	return ErrUserRolesUnsupported
}
