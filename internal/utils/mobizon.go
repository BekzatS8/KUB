package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type Client struct {
	ApiKey string
	Sender string // опционально
	DryRun bool   // dry-run режим
}

type SendSMSResponse struct {
	Code int `json:"code"`
	Data struct {
		MessageID string `json:"messageId"`
	} `json:"data"`
}

func NewClient(apiKey string) *Client {
	return &Client{ApiKey: apiKey}
}

func NewClientWithOptions(apiKey, sender string, dryRun bool) *Client {
	return &Client{ApiKey: apiKey, Sender: sender, DryRun: dryRun}
}

// SendSMS — отправка SMS через Mobizon (или имитация в dry-run)
func (c *Client) SendSMS(to, text string) (*SendSMSResponse, error) {
	// DRY-RUN: не делаем HTTP-запрос
	if c.DryRun || c.ApiKey == "" || c.ApiKey == "dry-run" {
		fmt.Printf("📩 [Mobizon][dry-run] to=%s sender=%q text=%q\n", to, c.Sender, text)
		return &SendSMSResponse{Code: 0}, nil
	}

	apiURL := "https://api.mobizon.kz/service/message/sendsmsmessage"

	form := url.Values{
		"apiKey":    {c.ApiKey},
		"recipient": {to},
		"text":      {text},
	}
	if c.Sender != "" {
		form.Set("from", c.Sender) // Если нужно указать Sender ID
	}

	resp, err := http.PostForm(apiURL, form)
	if err != nil {
		return nil, fmt.Errorf("send SMS request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println("📩 Mobizon raw response:", string(body))
	fmt.Printf("📤 Отправка на номер: %s (sender=%q)\n", to, c.Sender)

	var result SendSMSResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("mobizon returned error code: %d", result.Code)
	}
	return &result, nil
}
