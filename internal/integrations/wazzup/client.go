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
	SendMessage(ctx context.Context, apiKey string, req SendMessageRequest) (*SendMessageResponse, error)
}

type UserUpsert struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CreateIframeRequest struct {
	User  UserUpsert `json:"user"`
	Scope string     `json:"scope"`
}

type SendMessageRequest struct {
	ChannelID string `json:"channelId,omitempty"`
	ChatID    string `json:"chatId"`
	Text      string `json:"text"`
}

type SendMessageResponse struct {
	MessageID string
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
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("new request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
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
