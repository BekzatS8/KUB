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
}

type SendSMSResponse struct {
	Code int `json:"code"`
	Data struct {
		MessageID string `json:"messageId"`
	} `json:"data"`
}

// NewClient — инициализация клиента
func NewClient(apiKey string) *Client {
	return &Client{ApiKey: apiKey}
}

// SendSMS — отправка SMS
func (c *Client) SendSMS(to, text string) (*SendSMSResponse, error) {
	if c.ApiKey == "" {
		fmt.Printf("not api key")
	}

	apiURL := "https://api.mobizon.kz/service/message/sendsmsmessage"

	form := url.Values{
		"apiKey":    {c.ApiKey},
		"recipient": {to},
		"text":      {text},
		// "from":   {sender},
	}

	resp, err := http.PostForm(apiURL, form)
	if err != nil {
		return nil, fmt.Errorf("send SMS request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result SendSMSResponse
	fmt.Println("📩 Mobizon raw response:", string(body))
	fmt.Printf("📤 Отправка на номер: %s\n", to)

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("mobizon returned error code: %d", result.Code)
	}
	return &result, nil
}
