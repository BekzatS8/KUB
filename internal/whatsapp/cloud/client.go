package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL       string
	PhoneNumberID string
	AccessToken   string
	HTTPClient    *http.Client
}

func NewClient(baseURL, phoneNumberID, accessToken string) *Client {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return &Client{
		BaseURL:       base,
		PhoneNumberID: strings.TrimSpace(phoneNumberID),
		AccessToken:   strings.TrimSpace(accessToken),
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) SendTemplate(ctx context.Context, toE164, templateName, lang string, components []any) error {
	endpoint := fmt.Sprintf("%s/%s/messages", c.BaseURL, c.PhoneNumberID)
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                toE164,
		"type":              "template",
		"template": map[string]any{
			"name": templateName,
			"language": map[string]any{
				"code": lang,
			},
			"components": components,
		},
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("whatsapp cloud request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("whatsapp cloud status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return nil
}
