package wazzup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode"

	"turcompany/internal/repositories"
)

var (
	ErrUnauthorized = errors.New("wazzup unauthorized")
	ErrNotFound     = errors.New("wazzup integration not found")
	ErrBadPayload   = errors.New("wazzup bad payload")
	ErrDisabled     = errors.New("wazzup integration disabled")
	ErrBadRequest   = errors.New("wazzup bad request")
	ErrUpstream     = errors.New("wazzup upstream error")
	ErrUsersSync    = errors.New("wazzup users sync failed")
)

type Service struct {
	repo   repositories.WazzupRepository
	client Client
}

type SetupResponse struct {
	WebhookURL   string `json:"webhook_url"`
	WebhookToken string `json:"webhook_token"`
	CRMKey       string `json:"crm_key"`
}

func NewService(repo repositories.WazzupRepository, client Client) *Service {
	return &Service{repo: repo, client: client}
}

func (s *Service) Setup(ctx context.Context, ownerUserID int, webhooksBaseURL string, apiKey string, enabled bool) (*SetupResponse, error) {
	base := strings.TrimRight(strings.TrimSpace(webhooksBaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("%w: webhooks base url is required", ErrBadRequest)
	}
	crmKey, crmHash, err := generateCRMKey()
	if err != nil {
		return nil, err
	}

	integration, err := s.repo.GetIntegrationByOwnerUserID(ctx, ownerUserID)
	if err != nil {
		return nil, err
	}
	webhookToken := ""
	if integration != nil {
		webhookToken = integration.WebhookToken
	}

	if webhookToken == "" {
		_, webhookToken, err = s.repo.UpsertIntegrationByOwner(ctx, ownerUserID, strings.TrimSpace(apiKey), crmHash, "", enabled)
		if err != nil {
			return nil, err
		}
	}

	webhooksURI := base + "/integrations/wazzup/webhook/" + webhookToken
	_, webhookToken, err = s.repo.UpsertIntegrationByOwner(ctx, ownerUserID, strings.TrimSpace(apiKey), crmHash, webhooksURI, enabled)
	if err != nil {
		return nil, err
	}

	if enabled {
		log.Printf("[WAZZUP][setup] patching webhooks uri=%s token=%s crm_key=%s", webhooksURI, keyPrefix(apiKey), keyPrefix(crmKey))
		if err := s.client.PatchWebhooks(ctx, apiKey, webhooksURI, crmKey); err != nil {
			log.Printf("[WAZZUP][setup] patch webhooks failed err=%v", err)
			return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
		}
	}
	return &SetupResponse{WebhookURL: webhooksURI, WebhookToken: webhookToken, CRMKey: crmKey}, nil
}

func (s *Service) GetIframeURL(ctx context.Context, ownerUserID int, companyID int, userName string, _ string, _ int, _ int) (string, error) {
	integration, err := s.repo.GetIntegrationByOwnerUserID(ctx, ownerUserID)
	if err != nil {
		return "", err
	}
	if integration == nil {
		return "", ErrNotFound
	}
	if !integration.Enabled {
		return "", ErrDisabled
	}
	name := strings.TrimSpace(userName)
	if name == "" {
		crmUser, userErr := s.repo.GetCRMUserByID(ctx, ownerUserID)
		if userErr != nil {
			return "", userErr
		}
		if crmUser != nil {
			name = strings.TrimSpace(crmUser.Name)
		}
	}
	if name == "" {
		name = fmt.Sprintf("User %d", ownerUserID)
	}
	if companyID <= 0 {
		companyID = ownerUserID
	}
	wazzupUserID := fmt.Sprintf("kub-%d-%d", companyID, ownerUserID)
	if err := s.client.UpsertUsers(ctx, integration.APIKeyEnc, []UserUpsert{{ID: wazzupUserID, Name: name}}); err != nil {
		log.Printf("[WAZZUP][iframe] users sync failed owner_user_id=%d status_body_err=%v", ownerUserID, err)
		return "", fmt.Errorf("%w: %v", ErrUsersSync, ErrUpstream)
	}
	url, err := s.client.CreateIframe(ctx, integration.APIKeyEnc, CreateIframeRequest{
		User:  UserUpsert{ID: wazzupUserID, Name: name},
		Scope: "global",
	})
	if err != nil {
		log.Printf("[WAZZUP][iframe] create iframe failed owner_user_id=%d status_body_err=%v", ownerUserID, err)
		return "", fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	return url, nil
}

func (s *Service) HandleWebhook(ctx context.Context, token string, authHeader string, payload []byte) (int, bool, error) {
	start := time.Now()
	integration, err := s.repo.GetIntegrationByToken(ctx, token)
	if err != nil {
		return 0, false, err
	}
	if integration == nil {
		return 0, false, ErrNotFound
	}
	if !integration.Enabled {
		return 0, false, ErrDisabled
	}
	var req webhookPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return 0, false, ErrBadPayload
	}
	if !validateCRMKey(authHeader, integration.CRMKeyHash) {
		if req.Test || len(req.Messages) == 0 {
			return 0, false, nil
		}
		return 0, false, ErrUnauthorized
	}
	processed := 0
	created := false
	createdLeadID := 0
	for _, m := range req.Messages {
		chatType := strings.ToLower(strings.TrimSpace(firstNonEmpty(m.ChatType, m.ChannelType)))
		if chatType != "whatsapp" {
			continue
		}
		if isOutgoing(m) {
			continue
		}
		extID := strings.TrimSpace(m.ID)
		if extID == "" {
			continue
		}
		isNew, err := s.repo.RegisterDedup(ctx, integration.ID, extID)
		if err != nil {
			return 0, false, err
		}
		if !isNew {
			continue
		}
		phone := normalizePhone(m.ChatID)
		if phone == "" {
			continue
		}
		leadID, err := s.repo.FindLeadByPhone(ctx, phone)
		if err != nil {
			return 0, false, err
		}
		text := strings.TrimSpace(m.Text)
		if leadID == 0 {
			leadID, err = s.repo.CreateLeadFromInbound(ctx, integration.OwnerUserID, phone, text)
			if err != nil {
				return 0, false, err
			}
			created = true
			createdLeadID = leadID
		} else {
			if err := s.repo.UpdateLeadDescriptionIfEmpty(ctx, leadID, text); err != nil {
				return 0, false, err
			}
		}
		processed++
	}
	log.Printf("[WAZZUP:WEBHOOK] token=%s integration_id=%d processed_count=%d duration_ms=%d", tokenPrefix(token), integration.ID, processed, time.Since(start).Milliseconds())
	return createdLeadID, created, nil
}

type webhookPayload struct {
	Messages []webhookMessage `json:"messages"`
	Test     bool             `json:"test"`
}

type webhookMessage struct {
	ID          string `json:"id"`
	ChatID      string `json:"chatId"`
	ChatType    string `json:"chatType"`
	ChannelType string `json:"channelType"`
	Text        string `json:"text"`
	IsIncoming  *bool  `json:"isIncoming"`
	FromMe      *bool  `json:"fromMe"`
	Direction   string `json:"direction"`
}

func normalizePhone(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return ""
	}
	b := strings.Builder{}
	b.Grow(len(trimmed))
	for _, r := range trimmed {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func generateCRMKey() (plain string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate crm key: %w", err)
	}
	plain = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(plain))
	hash = hex.EncodeToString(sum[:])
	return plain, hash, nil
}

func validateCRMKey(authHeader, expectedHash string) bool {
	authHeader = strings.TrimSpace(authHeader)
	if !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return false
	}
	plain := strings.TrimSpace(authHeader[7:])
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:]) == strings.TrimSpace(expectedHash)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func isOutgoing(m webhookMessage) bool {
	if m.FromMe != nil {
		return *m.FromMe
	}
	if m.IsIncoming != nil {
		return !*m.IsIncoming
	}
	d := strings.ToLower(strings.TrimSpace(m.Direction))
	return d == "out" || d == "outgoing"
}

func tokenPrefix(token string) string {
	t := strings.TrimSpace(token)
	if len(t) > 8 {
		return t[:8]
	}
	return t
}

func keyPrefix(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	if len(v) > 6 {
		return v[:6] + "***"
	}
	return v + "***"
}
