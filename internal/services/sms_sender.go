package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrSMSSendDisabled    = errors.New("sms sender disabled")
	ErrSMSSendFailed      = errors.New("sms sender failed")
	ErrSMSAPIKeyMissing   = errors.New("sms api key missing")
	ErrSMSInvalidPhone    = errors.New("sms recipient phone is invalid")
	ErrSMSEmptyText       = errors.New("sms text is empty")
	ErrSMSTimeout         = errors.New("sms request timeout")
	ErrSMSProviderFailure = errors.New("sms provider returned error")
)

const (
	defaultMobizonAPIURL     = "https://api.mobizon.kz/service"
	defaultMobizonRequestURI = "/message/sendsmsmessage"
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
		cfg.BaseURL = defaultMobizonAPIURL
	}
	if strings.TrimSpace(cfg.ProviderName) == "" {
		cfg.ProviderName = "mobizon"
	}
	if strings.TrimSpace(cfg.RequestPath) == "" {
		cfg.RequestPath = defaultMobizonRequestURI
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
	to, err := normalizeSMSRecipient(msg.To)
	if err != nil {
		log.Printf("[sms][%s][send] status=invalid_recipient to=%s", m.cfg.ProviderName, redactPhoneForLog(msg.To))
		return nil, err
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return nil, ErrSMSEmptyText
	}
	if m.cfg.DryRun {
		log.Printf("[sms][%s][dry_run] to=%s text_len=%d", m.cfg.ProviderName, redactPhoneForLog(to), len(text))
		return &SMSResult{Provider: m.cfg.ProviderName, ProviderMessageID: "dry-run"}, nil
	}
	apiKey := strings.TrimSpace(m.cfg.APIKey)
	if apiKey == "" {
		return nil, ErrSMSAPIKeyMissing
	}
	endpoint, err := mobizonEndpointURL(m.cfg.BaseURL, m.cfg.RequestPath, apiKey)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid api url: %v", ErrSMSSendFailed, err)
	}

	form := url.Values{}
	form.Set("recipient", to)
	form.Set("text", text)
	if from := strings.TrimSpace(m.cfg.From); from != "" {
		form.Set("from", from)
	}
	requestBody := form.Encode()

	var lastErr error
	for attempt := 0; attempt <= m.cfg.Retries; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(requestBody))
		if reqErr != nil {
			return nil, fmt.Errorf("%w: build request: %v", ErrSMSSendFailed, reqErr)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Cache-Control", "no-cache")

		resp, doErr := m.httpClient.Do(req)
		if doErr != nil {
			if isTimeoutError(doErr) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				lastErr = fmt.Errorf("%w: %v", ErrSMSTimeout, doErr)
			} else {
				lastErr = fmt.Errorf("%w: request failed: %v", ErrSMSSendFailed, doErr)
			}
			if attempt < m.cfg.Retries {
				continue
			}
			break
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if resp.StatusCode >= http.StatusBadRequest {
			lastErr = fmt.Errorf("%w: http %d", ErrSMSProviderFailure, resp.StatusCode)
			if attempt < m.cfg.Retries && shouldRetrySMSStatus(resp.StatusCode) {
				continue
			}
			break
		}
		providerID, providerErr := parseMobizonResponse(body)
		if providerErr != nil {
			lastErr = providerErr
			break
		}
		log.Printf("[sms][%s][send] status=ok to=%s provider_message_id=%s text_len=%d", m.cfg.ProviderName, redactPhoneForLog(to), providerID, len(text))
		return &SMSResult{Provider: m.cfg.ProviderName, ProviderMessageID: providerID}, nil
	}
	log.Printf("[sms][%s][send] status=failed to=%s attempts=%d err=%v", m.cfg.ProviderName, redactPhoneForLog(to), m.cfg.Retries+1, lastErr)
	return nil, lastErr
}

type mobizonSendResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func parseMobizonResponse(body []byte) (string, error) {
	if len(body) == 0 {
		return "", fmt.Errorf("%w: empty response", ErrSMSProviderFailure)
	}
	var payload mobizonSendResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("%w: invalid json response", ErrSMSProviderFailure)
	}
	if payload.Code != 0 {
		msg := strings.TrimSpace(payload.Message)
		if msg == "" {
			msg = "provider rejected sms"
		}
		return "", fmt.Errorf("%w: code=%d message=%s", ErrSMSProviderFailure, payload.Code, msg)
	}
	return parseMobizonMessageID(payload.Data), nil
}

func parseMobizonMessageID(body []byte) string {
	if len(body) == 0 || string(body) == "null" {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return ""
	}
	for _, key := range []string{"messageId", "id"} {
		switch val := data[key].(type) {
		case string:
			return strings.TrimSpace(val)
		case float64:
			return strconv.FormatInt(int64(val), 10)
		case int:
			return strconv.Itoa(val)
		}
	}
	return ""
}

func mobizonEndpointURL(apiURL, requestPath, apiKey string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(apiURL))
	if err != nil {
		return "", err
	}
	if base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("absolute URL is required")
	}
	basePath := strings.TrimRight(base.EscapedPath(), "/")
	path := "/" + strings.TrimLeft(strings.TrimSpace(requestPath), "/")
	if path == "/" {
		path = defaultMobizonRequestURI
	}
	if strings.HasPrefix(strings.ToLower(path), "/service/") && strings.HasSuffix(strings.ToLower(basePath), "/service") {
		path = strings.TrimPrefix(path, "/service")
	}
	if basePath == "" && strings.Contains(strings.ToLower(base.Hostname()), "mobizon") && !strings.HasPrefix(strings.ToLower(path), "/service/") {
		basePath = "/service"
	}
	base.Path = basePath + path
	query := base.Query()
	query.Set("output", "json")
	query.Set("api", "v1")
	query.Set("apiKey", strings.TrimSpace(apiKey))
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func normalizeSMSRecipient(phone string) (string, error) {
	raw := strings.TrimSpace(phone)
	if raw == "" {
		return "", ErrSMSInvalidPhone
	}
	var b strings.Builder
	for i, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0:
			continue
		case r == ' ' || r == '-' || r == '(' || r == ')':
			continue
		default:
			return "", fmt.Errorf("%w: use international digits without plus", ErrSMSInvalidPhone)
		}
	}
	normalized := b.String()
	if len(normalized) == 11 && strings.HasPrefix(normalized, "8") {
		normalized = "7" + normalized[1:]
	}
	if len(normalized) < 7 || len(normalized) > 15 || strings.HasPrefix(normalized, "0") {
		return "", fmt.Errorf("%w: use international digits without plus", ErrSMSInvalidPhone)
	}
	return normalized, nil
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func shouldRetrySMSStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func redactPhoneForLog(phone string) string {
	p := strings.TrimSpace(phone)
	if len(p) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(p)-4) + p[len(p)-4:]
}
