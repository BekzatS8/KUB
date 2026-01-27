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

// WhatsAppClient sends messages via Meta WhatsApp Cloud API (Graph API).
// It keeps the same method signature as other SMS clients (SendSMS) so your services don't change.
//
// IMPORTANT
// 1) If you want to start a conversation (OTP before user wrote to you), use an APPROVED TEMPLATE.
// 2) Phone must be in E.164 digits ("+7..." -> "7...").
// 3) Billing is on Meta side (payment method in Business Manager), unless you use a BSP.
type WhatsAppClient struct {
	AccessToken   string
	PhoneNumberID string
	GraphBaseURL  string
	TemplateName  string
	LangCode      string
	DryRun        bool
	HTTPClient    *http.Client
}

func NewWhatsAppClient(accessToken, phoneNumberID, graphBaseURL, templateName, langCode string, dryRun bool) *WhatsAppClient {
	if strings.TrimSpace(graphBaseURL) == "" {
		graphBaseURL = "https://graph.facebook.com/v20.0"
	}
	if strings.TrimSpace(langCode) == "" {
		langCode = "ru"
	}
	return &WhatsAppClient{
		AccessToken:   strings.TrimSpace(accessToken),
		PhoneNumberID: strings.TrimSpace(phoneNumberID),
		GraphBaseURL:  strings.TrimRight(strings.TrimSpace(graphBaseURL), "/"),
		TemplateName:  strings.TrimSpace(templateName),
		LangCode:      strings.TrimSpace(langCode),
		DryRun:        dryRun,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

var (
	reDigits    = regexp.MustCompile(`\d{4,8}`)
	reNonDigits = regexp.MustCompile(`\D+`)
)

// SanitizeE164Digits приводит номер к формату E.164 без плюса
func SanitizeE164Digits(phone string) (string, error) {
	p := strings.TrimSpace(phone)
	p = reNonDigits.ReplaceAllString(p, "")
	if p == "" {
		return "", errors.New("empty phone")
	}

	// Для Казахстана: заменяем 8 на 7
	if len(p) == 11 && strings.HasPrefix(p, "8") {
		p = "7" + p[1:]
	}

	// Удаляем префикс + если есть
	if strings.HasPrefix(p, "+") {
		p = p[1:]
	}

	// Проверяем длину
	if len(p) < 11 || len(p) > 15 {
		return "", fmt.Errorf("invalid phone length: %d", len(p))
	}

	return p, nil
}

func extractOTP(text string) string {
	all := reDigits.FindAllString(text, -1)
	if len(all) == 0 {
		return ""
	}
	for _, s := range all {
		if len(s) == 6 {
			return s
		}
	}
	return all[0]
}

// SendSMS sends an OTP message.
// In your existing SMS_Service you pass text like: "Код подтверждения: 123456".
// Here we extract the digits and send them as a template parameter.
func (c *WhatsAppClient) SendSMS(to, text string) (*SendSMSResponse, error) {
	if c == nil {
		return nil, errors.New("whatsapp client is nil")
	}
	if !c.DryRun {
		if strings.TrimSpace(c.AccessToken) == "" || strings.TrimSpace(c.PhoneNumberID) == "" {
			return nil, errors.New("whatsapp configuration is incomplete")
		}
		if strings.TrimSpace(c.TemplateName) == "" {
			return nil, errors.New("whatsapp template is required")
		}
	}
	toDigits, err := SanitizeE164Digits(to)
	if err != nil {
		return nil, err
	}

	otp := extractOTP(text)
	paramText := otp
	if paramText == "" {
		paramText = strings.TrimSpace(text)
	}

	// DRY-RUN
	if c.DryRun {
		fmt.Printf("📩 [WhatsApp][dry-run] to=%s template=%q lang=%q param=%q full_text=%q\n",
			toDigits, c.TemplateName, c.LangCode, paramText, text)
		return &SendSMSResponse{Code: 0}, nil
	}

	endpoint := fmt.Sprintf("%s/%s/messages", c.GraphBaseURL, c.PhoneNumberID)

	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                toDigits,
	}

	// Template sending (required for OTP)
	payload["type"] = "template"
	payload["template"] = map[string]any{
		"name": c.TemplateName,
		"language": map[string]any{
			"code": c.LangCode,
		},
		"components": []any{
			map[string]any{
				"type": "body",
				"parameters": []any{
					map[string]any{"type": "text", "text": paramText},
				},
			},
		},
	}

	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("whatsapp request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("whatsapp api status=%d body=%s", resp.StatusCode, string(body))
	}

	// Try to extract message id (optional)
	type waResp struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	var wr waResp
	_ = json.Unmarshal(body, &wr)

	out := &SendSMSResponse{Code: 0}
	if len(wr.Messages) > 0 {
		out.Data.MessageID = wr.Messages[0].ID
	}
	return out, nil
}
