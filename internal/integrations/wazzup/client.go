package wazzup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.wazzup24.com"

type Client interface {
	PatchWebhooks(ctx context.Context, apiKey, webhooksURI, crmKey string) error
	CreateIframe(ctx context.Context, apiKey, phoneDigits string) (string, error)
}

type HTTPClient struct {
	baseURL string
	http    *http.Client
}

func NewHTTPClientFromEnv() *HTTPClient {
	baseURL := strings.TrimSpace(os.Getenv("WAZZUP_BASE_URL"))
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *HTTPClient) PatchWebhooks(ctx context.Context, apiKey, webhooksURI, crmKey string) error {
	payload := map[string]any{
		"webhooksUri":         webhooksURI,
		"messagesAndStatuses": true,
		"channelsUpdates":     true,
		"crmKey":              crmKey,
	}
	_, err := c.doJSON(ctx, http.MethodPatch, "/v3/webhooks", apiKey, payload)
	return err
}

func (c *HTTPClient) CreateIframe(ctx context.Context, apiKey, phoneDigits string) (string, error) {
	payload := map[string]any{
		"scope": "card",
		"filter": []map[string]string{{
			"chatType": "whatsapp",
			"chatId":   phoneDigits,
		}},
		"activeChat": map[string]string{
			"chatType": "whatsapp",
			"chatId":   phoneDigits,
		},
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

func (c *HTTPClient) doJSON(ctx context.Context, method, path, apiKey string, payload any) ([]byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wazzup request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("wazzup %s %s failed: status=%d body=%s", method, path, resp.StatusCode, string(body))
	}
	return body, nil
}
