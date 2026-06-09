package wazzup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	ChatID    string `json:"chatId"`
	Text      string `json:"text"`
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
	if req.ChatID == "" || req.Text == "" {
		return nil, fmt.Errorf("chatId and text are required")
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
	case "ig", "instagram", "instagram_direct", "direct":
		return "instagram"
	default:
		return v
	}
}
