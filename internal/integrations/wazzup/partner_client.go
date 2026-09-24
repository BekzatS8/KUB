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

const defaultPartnerBaseURL = "https://tech.wazzup24.com"

// TokenProvider отдаёт актуальный client_access_token дочернего аккаунта.
// Реализуется WhiteLabelClient.
type TokenProvider interface {
	AccessToken(ctx context.Context) (string, error)
}

// partnerWebhookEvents — события, на которые подписывается CRM.
//
// message.add даёт входящие (лиды и переписку), message.status_update —
// доставку исходящих, channel.* — статусы каналов и QR при подключении.
// Подписываемся только на то, что реально обрабатываем (требование доки).
var partnerWebhookEvents = []string{
	"message.add",
	"message.status_update",
	"channel.status_update",
	"channel.create",
	"channel.qr_update",
}

// PartnerClient — реализация Client поверх Tech Partner API
// (tech.wazzup24.com/v2) для дочернего White Label аккаунта.
//
// Отличие от HTTPClient (User API v3): авторизация идёт не статическим apiKey,
// а короткоживущим client_access_token (час жизни), который выдаёт
// TokenProvider. Параметр apiKey во всех методах игнорируется — он есть только
// ради совместимости с интерфейсом Client.
type PartnerClient struct {
	baseURL string
	tokens  TokenProvider
	http    *http.Client
	retries int
	retryIn time.Duration
}

func NewPartnerClient(baseURL string, tokens TokenProvider, timeout time.Duration, retries int, retryDelay time.Duration) *PartnerClient {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultPartnerBaseURL
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if retries < 0 {
		retries = 0
	}
	if retryDelay <= 0 {
		retryDelay = 300 * time.Millisecond
	}
	return &PartnerClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		tokens:  tokens,
		http:    &http.Client{Timeout: timeout},
		retries: retries,
		retryIn: retryDelay,
	}
}

// partnerWebhookSub — подписка на событие в ответе GET /v2/webhooks.
type partnerWebhookSub struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Event string `json:"event"`
}

// PatchWebhooks приводит подписки дочернего аккаунта к нужному набору событий
// с нужным URL.
//
// В v3 это был один вызов PATCH /v3/webhooks с общим webhooksUri. В v2 каждое
// событие — отдельная подписка со своим id, поэтому сверяем текущее состояние
// со списком partnerWebhookEvents: чего нет — создаём, у чего разъехался URL —
// правим. Чужие подписки (на другие URL) не трогаем.
//
// crmKey не используется: v2 не передаёт секрет во входящих вебхуках, защита —
// неугадываемый токен в пути /integrations/wazzup/webhook/:token.
func (c *PartnerClient) PatchWebhooks(ctx context.Context, _, webhooksURI, _ string) error {
	webhooksURI = strings.TrimSpace(webhooksURI)
	if webhooksURI == "" {
		return fmt.Errorf("webhooks uri is required")
	}

	body, err := c.doJSON(ctx, http.MethodGet, "/v2/webhooks", nil)
	if err != nil {
		return fmt.Errorf("list webhooks: %w", err)
	}
	var existing struct {
		Data []partnerWebhookSub `json:"data"`
	}
	if err := json.Unmarshal(body, &existing); err != nil {
		return fmt.Errorf("decode webhooks list: %w", err)
	}
	byEvent := make(map[string]partnerWebhookSub, len(existing.Data))
	for _, sub := range existing.Data {
		byEvent[strings.TrimSpace(sub.Event)] = sub
	}

	toCreate := make([]map[string]any, 0, len(partnerWebhookEvents))
	toUpdate := make([]map[string]any, 0, len(partnerWebhookEvents))
	for _, event := range partnerWebhookEvents {
		sub, ok := byEvent[event]
		switch {
		case !ok:
			toCreate = append(toCreate, map[string]any{"url": webhooksURI, "event": event})
		case strings.TrimSpace(sub.URL) != webhooksURI:
			toUpdate = append(toUpdate, map[string]any{"id": sub.ID, "url": webhooksURI, "event": event})
		}
	}

	if len(toCreate) > 0 {
		if _, err := c.doJSON(ctx, http.MethodPost, "/v2/webhooks", map[string]any{"data": toCreate}); err != nil {
			return fmt.Errorf("subscribe webhooks: %w", err)
		}
	}
	if len(toUpdate) > 0 {
		if _, err := c.doJSON(ctx, http.MethodPatch, "/v2/webhooks", map[string]any{"data": toUpdate}); err != nil {
			return fmt.Errorf("update webhooks: %w", err)
		}
	}
	return nil
}

// UpsertUsers синхронизирует сотрудников CRM с дочерним аккаунтом.
// Матчинг на стороне Wazzup идёт по id: нет — заведут, есть — обновят.
func (c *PartnerClient) UpsertUsers(ctx context.Context, _ string, users []UserUpsert) error {
	if len(users) == 0 {
		return nil
	}
	items := make([]map[string]any, 0, len(users))
	for _, u := range users {
		id := strings.TrimSpace(u.ID)
		if id == "" {
			continue
		}
		items = append(items, map[string]any{"id": id, "name": strings.TrimSpace(u.Name)})
	}
	if len(items) == 0 {
		return nil
	}
	_, err := c.doJSON(ctx, http.MethodPost, "/v2/users", map[string]any{"users": items})
	return err
}

// CreateIframe возвращает ссылку на окно чатов Wazzup (POST /v2/iframe-links/chats).
func (c *PartnerClient) CreateIframe(ctx context.Context, _ string, req CreateIframeRequest) (string, error) {
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = "global"
	}
	// user_id плоским полем, НЕ вложенным объектом user{id}.
	//
	// В документации схема запроса и пример противоречат друг другу: таблица
	// рисует «user → id», а пример шлёт «user_id». Правдой оказался пример —
	// проверено на живом API: с объектом user провайдер отвечает 400
	// «property user should not exist; user_id should not be empty».
	payload := map[string]any{
		"scope":   scope,
		"user_id": strings.TrimSpace(req.User.ID),
	}
	if name := strings.TrimSpace(req.User.Name); name != "" {
		payload["author_name"] = name
	}
	// chats[] допустим только для scope=card — при global провайдер его отвергает.
	if scope == "card" && len(req.Filter) > 0 {
		chats := make([]map[string]any, 0, len(req.Filter))
		for _, chat := range req.Filter {
			item := map[string]any{"chat_type": normalizePartnerChatType(chat.ChatType)}
			if v := strings.TrimSpace(chat.ChatID); v != "" {
				item["chat_id"] = v
			}
			if v := strings.TrimSpace(chat.Name); v != "" {
				item["contact_name"] = v
			}
			chats = append(chats, item)
		}
		payload["chats"] = chats
	}
	if req.ActiveChat != nil {
		active := map[string]any{"chat_type": normalizePartnerChatType(req.ActiveChat.ChatType)}
		if v := strings.TrimSpace(req.ActiveChat.ChatID); v != "" {
			active["chat_id"] = v
		}
		if v := strings.TrimSpace(req.ActiveChat.ChannelID); v != "" {
			active["channel_id"] = v
		}
		payload["active_chat"] = active
	}

	body, err := c.doJSON(ctx, http.MethodPost, "/v2/iframe-links/chats", payload)
	if err != nil {
		return "", err
	}
	var resp struct {
		Data struct {
			Link string `json:"link"`
			URL  string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode iframe response: %w", err)
	}
	if link := strings.TrimSpace(resp.Data.Link); link != "" {
		return link, nil
	}
	if link := strings.TrimSpace(resp.Data.URL); link != "" {
		return link, nil
	}
	return "", fmt.Errorf("iframe url is empty")
}

// ListChannels отдаёт каналы дочернего аккаунта (GET /v2/channels).
//
// Форма ответа — {"data":[{channel_id, transport, state, status, phone, …}]}.
// Разбор переиспользует extractChannelItems/mapChannel: они уже понимают и
// channel_id, и status/state.
func (c *PartnerClient) ListChannels(ctx context.Context, _ string) ([]Channel, error) {
	body, err := c.doJSON(ctx, http.MethodGet, "/v2/channels", nil)
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

// SendMessage отправляет сообщение через POST /v2/messages.
//
// Отправка асинхронная: в ответе приходит request_id, а реальный message_id
// прилетит позже вебхуком message.status_update. Возвращаем request_id —
// вызывающий код использует его как внешний идентификатор.
func (c *PartnerClient) SendMessage(ctx context.Context, _ string, req SendMessageRequest) (*SendMessageResponse, error) {
	chatID := strings.TrimSpace(req.ChatID)
	text := strings.TrimSpace(req.Text)
	if chatID == "" || text == "" {
		return nil, fmt.Errorf("chatId and text are required")
	}
	channelID := strings.TrimSpace(req.ChannelID)
	if channelID == "" {
		return nil, fmt.Errorf("channelId is required for partner api")
	}

	recipient := map[string]any{
		"chat_type": normalizePartnerChatType(req.ChatType),
		"chat_id":   chatID,
	}
	payload := map[string]any{
		"channel_id": channelID,
		"recipient":  recipient,
		"text":       text,
	}
	if v := strings.TrimSpace(req.CRMUserID); v != "" {
		payload["crm_user_id"] = v
	}

	body, err := c.doJSON(ctx, http.MethodPost, "/v2/messages", payload)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			RequestID string `json:"request_id"`
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode send message response: %w", err)
	}
	id := strings.TrimSpace(resp.Data.MessageID)
	if id == "" {
		id = strings.TrimSpace(resp.Data.RequestID)
	}
	return &SendMessageResponse{MessageID: id}, nil
}

// normalizePartnerChatType приводит транспорт к chat_type партнёрского API.
// Внутри CRM транспорт зовётся whatsapp/telegram/instagram — в v2 это те же
// значения, но пустое поле недопустимо, поэтому дефолтимся в whatsapp.
func normalizePartnerChatType(value string) string {
	v := normalizeTransport(value)
	if v == "" {
		return "whatsapp"
	}
	return v
}

func (c *PartnerClient) doJSON(ctx context.Context, method, path string, payload any) ([]byte, error) {
	var raw []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		raw = b
	}

	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		token, err := c.tokens.AccessToken(ctx)
		if err != nil {
			return nil, fmt.Errorf("partner token: %w", err)
		}

		var body io.Reader
		if raw != nil {
			body = bytes.NewReader(raw)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
		if err != nil {
			return nil, fmt.Errorf("new request: %w", err)
		}
		if raw != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("wazzup partner request: %w", err)
		} else {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return respBody, nil
			}
			lastErr = fmt.Errorf("wazzup partner %s %s failed: status=%d body=%s", method, path, resp.StatusCode, string(respBody))
			// 401 — токен протух раньше срока: сбрасываем кэш и пробуем ещё раз.
			if resp.StatusCode == http.StatusUnauthorized {
				if invalidator, ok := c.tokens.(interface{ InvalidateToken() }); ok {
					invalidator.InvalidateToken()
				}
			} else if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
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
		lastErr = errors.New("wazzup partner request failed")
	}
	return nil, lastErr
}

// DeleteChannel удаляет канал в дочернем аккаунте (DELETE /v2/channels/{id}).
//
// Для White Label это единственный способ убрать канал: кабинета у дочернего
// аккаунта нет, и CRM — единственная точка управления. Операция необратима,
// поэтому переписку по умолчанию сохраняем (delete_chats=false).
func (c *PartnerClient) DeleteChannel(ctx context.Context, _, externalChannelID string, deleteChats bool) error {
	id := strings.TrimSpace(externalChannelID)
	if id == "" {
		return fmt.Errorf("channel id is required")
	}
	path := fmt.Sprintf("/v2/channels/%s?delete_chats=%t", url.PathEscape(id), deleteChats)
	_, err := c.doJSON(ctx, http.MethodDelete, path, nil)
	return err
}
