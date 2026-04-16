package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrSMSSendDisabled = errors.New("sms sender disabled")
	ErrSMSSendFailed   = errors.New("sms sender failed")
)

type SMSMessage struct {
	To   string
	Text string
}

type SMSResult struct {
	ProviderMessageID string
	Provider          string
}

type SMSSender interface {
	Send(ctx context.Context, msg SMSMessage) (*SMSResult, error)
}

type MobizonSMSConfig struct {
	Enabled      bool
	APIKey       string
	BaseURL      string
	From         string
	Timeout      time.Duration
	Retries      int
	DryRun       bool
	ProviderName string
	RequestPath  string
}

type MobizonSMSClient struct {
	httpClient *http.Client
	cfg        MobizonSMSConfig
}

func NewMobizonSMSClient(cfg MobizonSMSConfig) *MobizonSMSClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = "https://api.mobizon.kz"
	}
	if strings.TrimSpace(cfg.ProviderName) == "" {
		cfg.ProviderName = "mobizon"
	}
	if strings.TrimSpace(cfg.RequestPath) == "" {
		cfg.RequestPath = "/service/message/sendsmsmessage"
	}
	if cfg.Retries < 0 {
		cfg.Retries = 0
	}
	return &MobizonSMSClient{httpClient: &http.Client{Timeout: cfg.Timeout}, cfg: cfg}
}

func (m *MobizonSMSClient) Send(ctx context.Context, msg SMSMessage) (*SMSResult, error) {
	if m == nil {
		return nil, ErrSMSSendDisabled
	}
	if !m.cfg.Enabled {
		return nil, ErrSMSSendDisabled
	}
	to := strings.TrimSpace(msg.To)
	text := strings.TrimSpace(msg.Text)
	if to == "" || text == "" {
		return nil, fmt.Errorf("%w: phone and text are required", ErrSMSSendFailed)
	}
	if m.cfg.DryRun {
		log.Printf("[sms][%s][dry_run] to=%s text_len=%d", m.cfg.ProviderName, redactPhoneForLog(to), len(text))
		return &SMSResult{Provider: m.cfg.ProviderName, ProviderMessageID: "dry-run"}, nil
	}
	apiKey := strings.TrimSpace(m.cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: api key is empty", ErrSMSSendFailed)
	}
	base, err := url.Parse(strings.TrimSpace(m.cfg.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base url: %v", ErrSMSSendFailed, err)
	}
	base.Path = strings.TrimRight(base.Path, "/") + m.cfg.RequestPath

	payload := map[string]string{"recipient": to, "text": text}
	if from := strings.TrimSpace(m.cfg.From); from != "" {
		payload["from"] = from
	}
	requestBody, _ := json.Marshal(payload)

	var lastErr error
	for attempt := 0; attempt <= m.cfg.Retries; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewReader(requestBody))
		if reqErr != nil {
			return nil, fmt.Errorf("%w: build request: %v", ErrSMSSendFailed, reqErr)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, doErr := m.httpClient.Do(req)
		if doErr != nil {
			lastErr = fmt.Errorf("request failed: %w", doErr)
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if resp.StatusCode >= http.StatusBadRequest {
			lastErr = fmt.Errorf("http %d", resp.StatusCode)
			continue
		}
		providerID := parseMobizonMessageID(body)
		return &SMSResult{Provider: m.cfg.ProviderName, ProviderMessageID: providerID}, nil
	}
	return nil, fmt.Errorf("%w: %v", ErrSMSSendFailed, lastErr)
}

func parseMobizonMessageID(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if data, ok := payload["data"].(map[string]any); ok {
		if val, ok := data["messageId"].(string); ok {
			return strings.TrimSpace(val)
		}
		if val, ok := data["id"].(string); ok {
			return strings.TrimSpace(val)
		}
	}
	if val, ok := payload["messageId"].(string); ok {
		return strings.TrimSpace(val)
	}
	return ""
}

func redactPhoneForLog(phone string) string {
	p := strings.TrimSpace(phone)
	if len(p) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(p)-4) + p[len(p)-4:]
}
