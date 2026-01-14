package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// GreenAPIClient отправляет сообщения через GreenAPI WhatsApp API
type GreenAPIClient struct {
	URL              string
	IDInstance       string
	APITokenInstance string
	DryRun           bool
	HTTPClient       *http.Client
}

// NewGreenAPIClient создаёт клиент GreenAPI
func NewGreenAPIClient(url, idInstance, apiTokenInstance string, dryRun bool) *GreenAPIClient {
	return &GreenAPIClient{
		URL:              strings.TrimSpace(url),
		IDInstance:       strings.TrimSpace(idInstance),
		APITokenInstance: strings.TrimSpace(apiTokenInstance),
		DryRun:           dryRun,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

var reNonDigits = regexp.MustCompile(`\D+`)

// sanitizePhoneForGreenAPI преобразует номер в формат GreenAPI: 79991234567@c.us
func sanitizePhoneForGreenAPI(phone string) (string, error) {
	p := strings.TrimSpace(phone)
	p = reNonDigits.ReplaceAllString(p, "")
	if p == "" {
		return "", errors.New("empty phone")
	}
	// 8XXXXXXXXXX -> 7XXXXXXXXXX
	if len(p) == 11 && strings.HasPrefix(p, "8") {
		p = "7" + p[1:]
	}
	if len(p) < 11 || len(p) > 15 {
		return "", fmt.Errorf("invalid phone length: %d", len(p))
	}
	return p + "@c.us", nil
}

// SendSMS отправляет текстовое сообщение через GreenAPI
func (c *GreenAPIClient) SendSMS(to, text string) (*SendSMSResponse, error) {
	if c == nil {
		return nil, errors.New("greenapi client is nil")
	}

	if !c.DryRun {
		if strings.TrimSpace(c.URL) == "" ||
			strings.TrimSpace(c.IDInstance) == "" ||
			strings.TrimSpace(c.APITokenInstance) == "" {
			return nil, errors.New("greenapi configuration is incomplete")
		}
	}

	chatID, err := sanitizePhoneForGreenAPI(to)
	if err != nil {
		return nil, fmt.Errorf("invalid phone for greenapi: %w", err)
	}

	// DRY-RUN режим
	if c.DryRun {
		fmt.Printf("📩 [GreenAPI][dry-run] to=%s chatId=%s text=%q\n",
			to, chatID, text)
		return &SendSMSResponse{Code: 0}, nil
	}

	// Формируем URL для отправки сообщения
	endpoint := fmt.Sprintf("%s/waInstance%s/sendMessage/%s",
		c.URL, c.IDInstance, c.APITokenInstance)

	payload := map[string]interface{}{
		"chatId":  chatID,
		"message": text,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("greenapi request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("greenapi api status=%d body=%s",
			resp.StatusCode, string(body))
	}

	// Разбираем ответ GreenAPI
	type greenAPIResponse struct {
		IDMessage string `json:"idMessage"`
	}

	var gr greenAPIResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		// Если не можем распарсить ID, но статус 200 - всё равно считаем успехом
		fmt.Printf("[GreenAPI] warning: cannot parse response: %v\n", err)
	}

	out := &SendSMSResponse{Code: 0}
	if gr.IDMessage != "" {
		out.Data.MessageID = gr.IDMessage
	}

	return out, nil
}
