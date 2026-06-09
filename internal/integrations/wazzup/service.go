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

	"turcompany/internal/models"
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
	repo               repositories.WazzupRepository
	client             Client
	defaultAPIToken    string
	defaultChannelID   string
	webhookVerifyToken string
	webhookBaseURL     string
}

type SetupResponse struct {
	WebhookURL   string `json:"webhook_url"`
	WebhookToken string `json:"webhook_token"`
	CRMKey       string `json:"crm_key"`
}

type IframeOptions struct {
	Transport string `json:"transport,omitempty"`
	ChannelID string `json:"channel_id,omitempty"`
}

type IframeResponse struct {
	URL             string `json:"url"`
	IframeURL       string `json:"iframe_url"`
	ChannelSpecific bool   `json:"channel_specific"`
	Transport       string `json:"transport,omitempty"`
	ChannelID       string `json:"channel_id,omitempty"`
	Message         string `json:"message,omitempty"`
}

type SendDialogMessageResponse struct {
	Message *models.WazzupDialogMessage `json:"message"`
}

func NewService(repo repositories.WazzupRepository, client Client, defaultAPIToken, defaultChannelID, webhookVerifyToken, webhookBaseURL string) *Service {
	return &Service{
		repo:               repo,
		client:             client,
		defaultAPIToken:    strings.TrimSpace(defaultAPIToken),
		defaultChannelID:   strings.TrimSpace(defaultChannelID),
		webhookVerifyToken: strings.TrimSpace(webhookVerifyToken),
		webhookBaseURL:     strings.TrimSpace(webhookBaseURL),
	}
}

func (s *Service) Setup(ctx context.Context, ownerUserID int, webhooksBaseURL string, enabled bool) (*SetupResponse, error) {
	base := strings.TrimRight(strings.TrimSpace(webhooksBaseURL), "/")
	if base == "" {
		base = strings.TrimRight(s.webhookBaseURL, "/")
	}
	if base == "" {
		return nil, fmt.Errorf("%w: webhooks base url is required", ErrBadRequest)
	}
	apiKey := s.defaultAPIToken
	if enabled && strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("%w: wazzup api token is required", ErrBadRequest)
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
		log.Printf("integration=wazzup operation=setup owner_user_id=%d enabled=%v webhook_configured=%v", ownerUserID, enabled, webhooksURI != "")
		if err := s.client.PatchWebhooks(ctx, apiKey, webhooksURI, crmKey); err != nil {
			log.Printf("integration=wazzup operation=setup status=failed owner_user_id=%d err=%v", ownerUserID, err)
			return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
		}
	}
	return &SetupResponse{WebhookURL: webhooksURI, WebhookToken: webhookToken, CRMKey: crmKey}, nil
}

func (s *Service) GetIframeURL(ctx context.Context, ownerUserID int, companyID int, userName string) (string, error) {
	resp, err := s.GetIframe(ctx, ownerUserID, companyID, userName, IframeOptions{})
	if err != nil {
		return "", err
	}
	return resp.URL, nil
}

func (s *Service) GetIframe(ctx context.Context, ownerUserID int, companyID int, userName string, opts IframeOptions) (*IframeResponse, error) {
	integration, err := s.activeIntegrationForUser(ctx, ownerUserID)
	if err != nil {
		return nil, err
	}
	transport := normalizeTransport(opts.Transport)
	channelID := strings.TrimSpace(opts.ChannelID)
	if transport != "" && !isSupportedTransport(transport) {
		return nil, fmt.Errorf("%w: unsupported transport", ErrBadRequest)
	}
	if transport != "" || channelID != "" {
		ch, err := s.resolveIframeChannel(ctx, ownerUserID, integration.ID, transport, channelID)
		if err != nil {
			return nil, err
		}
		if ch == nil {
			return nil, ErrNotFound
		}
		channelID = strings.TrimSpace(ch.ExternalChannelID)
		transport = normalizeTransport(ch.Transport)
	}
	name := strings.TrimSpace(userName)
	if name == "" {
		crmUser, userErr := s.repo.GetCRMUserByID(ctx, ownerUserID)
		if userErr != nil {
			return nil, userErr
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
	apiKey := s.resolveAPIKey(integration.APIKeyEnc)
	if err := s.client.UpsertUsers(ctx, apiKey, []UserUpsert{{ID: wazzupUserID, Name: name}}); err != nil {
		log.Printf("integration=wazzup operation=iframe_upsert_users status=failed owner_user_id=%d err=%v", ownerUserID, err)
		return nil, fmt.Errorf("%w: %v", ErrUsersSync, ErrUpstream)
	}
	url, err := s.client.CreateIframe(ctx, apiKey, CreateIframeRequest{
		User:  UserUpsert{ID: wazzupUserID, Name: name},
		Scope: "global",
	})
	if err != nil {
		log.Printf("integration=wazzup operation=iframe_create status=failed owner_user_id=%d err=%v", ownerUserID, err)
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	resp := &IframeResponse{
		URL:       url,
		IframeURL: url,
		Transport: transport,
		ChannelID: channelID,
	}
	if transport != "" || channelID != "" {
		resp.Message = "Wazzup returned global iframe only"
	}
	return resp, nil
}

func (s *Service) resolveIframeChannel(ctx context.Context, ownerUserID, integrationID int, transport, channelID string) (*models.WazzupChannel, error) {
	channels, err := s.SyncChannels(ctx, ownerUserID)
	if err != nil {
		cached, cacheErr := s.repo.ListChannels(ctx, integrationID)
		if cacheErr != nil || len(cached) == 0 {
			return nil, err
		}
		channels = cached
	}
	var fallback *models.WazzupChannel
	for i := range channels {
		ch := channels[i]
		if strings.TrimSpace(channelID) != "" && strings.TrimSpace(ch.ExternalChannelID) != strings.TrimSpace(channelID) {
			continue
		}
		if transport != "" && normalizeTransport(ch.Transport) != transport {
			continue
		}
		if fallback == nil {
			fallback = &ch
		}
		if isActiveChannelStatus(ch.Status) {
			return &ch, nil
		}
	}
	return fallback, nil
}

func (s *Service) GetStatus(ctx context.Context, ownerUserID int) (*models.WazzupStatus, error) {
	status, err := s.repo.GetStatus(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.activeIntegrationForUser(ctx, ownerUserID); err != nil {
		status.IframeAvailable = false
		if errors.Is(err, ErrNotFound) {
			status.Configured = false
		}
		if errors.Is(err, ErrDisabled) {
			status.Enabled = false
		}
	}
	return status, nil
}

func (s *Service) SyncChannels(ctx context.Context, ownerUserID int) ([]models.WazzupChannel, error) {
	integration, err := s.activeIntegrationForUser(ctx, ownerUserID)
	if err != nil {
		return nil, err
	}
	apiKey := s.resolveAPIKey(integration.APIKeyEnc)
	providerChannels, err := s.client.ListChannels(ctx, apiKey)
	if err != nil {
		log.Printf("integration=wazzup operation=channels_sync status=failed owner_user_id=%d err=%v", ownerUserID, err)
		cached, cacheErr := s.repo.ListChannels(ctx, integration.ID)
		if cacheErr == nil && len(cached) > 0 {
			return cached, nil
		}
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}

	channels := make([]models.WazzupChannel, 0, len(providerChannels))
	for _, ch := range providerChannels {
		raw, _ := json.Marshal(ch.RawPayload)
		channels = append(channels, models.WazzupChannel{
			IntegrationID:     integration.ID,
			ExternalChannelID: strings.TrimSpace(ch.ID),
			Transport:         normalizeTransport(ch.Transport),
			Name:              strings.TrimSpace(ch.Name),
			Username:          strings.TrimSpace(ch.Username),
			Phone:             normalizePhone(ch.Phone),
			Status:            strings.ToLower(strings.TrimSpace(ch.Status)),
			Provider:          "wazzup",
			RawPayload:        raw,
		})
	}
	if err := s.repo.UpsertChannels(ctx, integration.ID, channels); err != nil {
		return nil, err
	}
	return s.repo.ListChannels(ctx, integration.ID)
}

func (s *Service) ListDialogs(ctx context.Context, userID int, transport string) ([]models.WazzupDialog, error) {
	if _, err := s.activeIntegrationForUser(ctx, userID); err != nil {
		return nil, err
	}
	return s.repo.ListExternalDialogs(ctx, userID, normalizeTransport(transport))
}

func (s *Service) ListDialogMessages(ctx context.Context, userID, dialogID, limit, offset int) ([]models.WazzupDialogMessage, error) {
	if _, err := s.activeIntegrationForUser(ctx, userID); err != nil {
		return nil, err
	}
	dialog, err := s.repo.GetExternalDialog(ctx, userID, dialogID)
	if err != nil {
		return nil, err
	}
	if dialog == nil {
		return nil, ErrNotFound
	}
	return s.repo.ListExternalMessages(ctx, userID, dialogID, limit, offset)
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
	if s.webhookVerifyToken != "" && strings.TrimSpace(authHeader) != "" {
		if !strings.EqualFold(strings.TrimSpace(authHeader), "Bearer "+s.webhookVerifyToken) &&
			strings.TrimSpace(authHeader) != s.webhookVerifyToken {
			return 0, false, ErrUnauthorized
		}
	}
	var req webhookPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return 0, false, ErrBadPayload
	}
	// Wazzup may not send Authorization header — rely on webhook token for auth.
	// If header IS present, validate it as an extra security check.
	if strings.TrimSpace(authHeader) != "" && !validateCRMKey(authHeader, integration.CRMKeyHash) {
		log.Printf("[WAZZUP][webhook] auth failed auth_header_len=%d", len(authHeader))
		if req.Test || len(req.Messages) == 0 {
			return 0, false, nil
		}
		return 0, false, ErrUnauthorized
	}
	processed := 0
	created := false
	createdLeadID := 0
	for _, m := range req.Messages {
		leadID, leadCreated, messageCreated, err := s.processIncomingWebhookMessage(ctx, integration, m)
		if err != nil {
			return 0, false, err
		}
		if leadCreated {
			created = true
			createdLeadID = leadID
		}
		if messageCreated {
			processed++
		}
	}
	log.Printf("integration=wazzup operation=webhook status=ok integration_id=%d processed=%d duration_ms=%d", integration.ID, processed, time.Since(start).Milliseconds())
	return createdLeadID, created, nil
}

func (s *Service) processIncomingWebhookMessage(ctx context.Context, integration *models.WazzupIntegration, m webhookMessage) (leadID int, leadCreated bool, messageCreated bool, err error) {
	if isOutgoing(m) {
		return 0, false, false, nil
	}
	transport := normalizeTransport(firstNonEmpty(m.Transport, m.ChatType, m.ChannelType))
	if !isSupportedTransport(transport) {
		return 0, false, false, nil
	}
	externalMessageID := strings.TrimSpace(firstNonEmpty(m.ID, m.MessageID))
	if externalMessageID == "" {
		return 0, false, false, nil
	}
	isNew, err := s.repo.RegisterDedup(ctx, integration.ID, externalMessageID)
	if err != nil {
		return 0, false, false, err
	}
	if !isNew {
		return 0, false, false, nil
	}

	externalChatID := strings.TrimSpace(firstNonEmpty(m.ChatID, m.ExternalChatID, m.ContactID))
	if externalChatID == "" {
		return 0, false, false, nil
	}
	channelID := strings.TrimSpace(firstNonEmpty(m.ChannelID, m.ChannelIDAlt, m.ChannelGuid, s.defaultChannelID))
	text := strings.TrimSpace(firstNonEmpty(m.Text, m.Body, m.Caption))
	createdAt := parseWebhookTime(firstNonEmpty(m.CreatedAt, m.DateTime, m.Timestamp))
	raw, _ := json.Marshal(m)

	var clientIDPtr *int
	var leadIDPtr *int
	phone := ""
	if transport == "whatsapp" {
		phone = normalizePhone(externalChatID)
		if phone != "" {
			clientID, err := s.repo.FindClientByPhone(ctx, phone)
			if err != nil {
				return 0, false, false, err
			}
			if clientID > 0 {
				clientIDPtr = &clientID
			}
			leadID, err = s.repo.FindLeadByPhone(ctx, phone)
			if err != nil {
				return 0, false, false, err
			}
			if leadID == 0 && clientID == 0 {
				leadID, err = s.repo.CreateLeadFromInbound(ctx, integration.OwnerUserID, phone, text)
				if err != nil {
					return 0, false, false, err
				}
				leadCreated = true
			} else if leadID > 0 {
				if err := s.repo.UpdateLeadDescriptionIfEmpty(ctx, leadID, text); err != nil {
					return 0, false, false, err
				}
			}
			if leadID > 0 {
				leadIDPtr = &leadID
			}
		}
	}

	displayName := strings.TrimSpace(firstNonEmpty(m.ChatName, m.ContactName, m.AuthorName, m.Username, phone, externalChatID))
	dialog, err := s.repo.UpsertExternalChat(ctx, repositories.ExternalChatUpsert{
		OwnerUserID:       integration.OwnerUserID,
		Transport:         transport,
		ExternalChatID:    externalChatID,
		ExternalChannelID: channelID,
		DisplayName:       displayName,
		Username:          strings.TrimSpace(m.Username),
		Phone:             phone,
		ClientID:          clientIDPtr,
		LeadID:            leadIDPtr,
		RawPayload:        raw,
		LastMessageAt:     createdAt,
		Direction:         "incoming",
	})
	if err != nil {
		return 0, false, false, err
	}
	if dialog == nil {
		return leadID, leadCreated, false, nil
	}
	_, messageCreated, err = s.repo.CreateExternalMessage(ctx, repositories.ExternalMessageCreate{
		ChatID:            dialog.ID,
		Transport:         transport,
		ExternalMessageID: externalMessageID,
		ExternalChannelID: channelID,
		Direction:         "incoming",
		Status:            "received",
		Text:              text,
		RawPayload:        raw,
		CreatedAt:         createdAt,
	})
	if err != nil {
		return 0, false, false, err
	}
	log.Printf("integration=wazzup operation=webhook_message status=ok provider=wazzup transport=%s message_created=%v", transport, messageCreated)
	return leadID, leadCreated, messageCreated, nil
}

func (s *Service) SendMessage(ctx context.Context, ownerUserID int, chatID, text string) (*SendMessageResponse, error) {
	integration, err := s.activeIntegrationForUser(ctx, ownerUserID)
	if err != nil {
		return nil, err
	}
	req := SendMessageRequest{
		ChannelID: s.defaultChannelID,
		ChatID:    strings.TrimSpace(chatID),
		Text:      strings.TrimSpace(text),
	}
	resp, err := s.client.SendMessage(ctx, s.resolveAPIKey(integration.APIKeyEnc), req)
	if err != nil {
		log.Printf("integration=wazzup operation=send_message status=failed owner_user_id=%d target_chat=%s err=%v", ownerUserID, maskChatID(chatID), err)
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	log.Printf("integration=wazzup operation=send_message status=ok owner_user_id=%d target_chat=%s message_id=%s", ownerUserID, maskChatID(chatID), tokenPrefix(resp.MessageID))
	return resp, nil
}

func (s *Service) SendDialogMessage(ctx context.Context, userID, dialogID int, text string) (*models.WazzupDialogMessage, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("%w: text is required", ErrBadRequest)
	}
	integration, err := s.activeIntegrationForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	dialog, err := s.repo.GetExternalDialog(ctx, userID, dialogID)
	if err != nil {
		return nil, err
	}
	if dialog == nil {
		return nil, ErrNotFound
	}
	if strings.TrimSpace(dialog.ExternalChatID) == "" {
		return nil, fmt.Errorf("%w: external chat id is required", ErrBadRequest)
	}
	channelID := strings.TrimSpace(firstNonEmpty(dialog.ExternalChannelID, s.defaultChannelID))
	if channelID == "" {
		return nil, fmt.Errorf("%w: channel id is required", ErrBadRequest)
	}

	resp, err := s.client.SendMessage(ctx, s.resolveAPIKey(integration.APIKeyEnc), SendMessageRequest{
		ChannelID: channelID,
		ChatID:    dialog.ExternalChatID,
		Text:      text,
	})
	if err != nil {
		log.Printf("integration=wazzup operation=dialog_send status=failed user_id=%d dialog_id=%d transport=%s err=%v", userID, dialogID, dialog.Transport, err)
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	now := time.Now()
	raw, _ := json.Marshal(map[string]string{"messageId": resp.MessageID})
	saved, _, err := s.repo.CreateExternalMessage(ctx, repositories.ExternalMessageCreate{
		ChatID:            dialog.ID,
		SenderID:          &userID,
		Transport:         dialog.Transport,
		ExternalMessageID: strings.TrimSpace(resp.MessageID),
		ExternalChannelID: channelID,
		Direction:         "outgoing",
		Status:            "sent",
		Text:              text,
		RawPayload:        raw,
		CreatedAt:         now,
	})
	if err != nil {
		return nil, err
	}
	_, err = s.repo.UpsertExternalChat(ctx, repositories.ExternalChatUpsert{
		OwnerUserID:       userID,
		Transport:         dialog.Transport,
		ExternalChatID:    dialog.ExternalChatID,
		ExternalChannelID: channelID,
		DisplayName:       dialog.DisplayName,
		Username:          dialog.Username,
		Phone:             dialog.Phone,
		ClientID:          dialog.ClientID,
		LeadID:            dialog.LeadID,
		LastMessageAt:     now,
		Direction:         "outgoing",
	})
	if err != nil {
		return nil, err
	}
	log.Printf("integration=wazzup operation=dialog_send status=ok user_id=%d dialog_id=%d transport=%s message_id=%s", userID, dialogID, dialog.Transport, tokenPrefix(resp.MessageID))
	return saved, nil
}

func (s *Service) activeIntegrationForUser(ctx context.Context, ownerUserID int) (*models.WazzupIntegration, error) {
	integration, err := s.repo.GetIntegrationByOwnerUserID(ctx, ownerUserID)
	if err != nil {
		return nil, err
	}
	if integration != nil && integration.Enabled {
		return integration, nil
	}

	shared, err := s.repo.GetAnyEnabledIntegration(ctx)
	if err != nil {
		return nil, err
	}
	if shared != nil {
		return shared, nil
	}
	if integration != nil {
		return nil, ErrDisabled
	}
	return nil, ErrNotFound
}

func (s *Service) resolveAPIKey(saved string) string {
	if strings.TrimSpace(s.defaultAPIToken) != "" {
		return s.defaultAPIToken
	}
	return strings.TrimSpace(saved)
}

func maskChatID(chatID string) string {
	clean := strings.TrimSpace(chatID)
	if len(clean) <= 4 {
		return "***"
	}
	return clean[:2] + "***" + clean[len(clean)-2:]
}

type webhookPayload struct {
	Messages []webhookMessage `json:"messages"`
	Test     bool             `json:"test"`
}

type webhookMessage struct {
	ID             string `json:"id"`
	MessageID      string `json:"messageId"`
	ChatID         string `json:"chatId"`
	ExternalChatID string `json:"externalChatId"`
	ContactID      string `json:"contactId"`
	ChatType       string `json:"chatType"`
	ChannelType    string `json:"channelType"`
	Transport      string `json:"transport"`
	ChannelID      string `json:"channelId"`
	ChannelIDAlt   string `json:"channel_id"`
	ChannelGuid    string `json:"channelGuid"`
	Text           string `json:"text"`
	Body           string `json:"body"`
	Caption        string `json:"caption"`
	ChatName       string `json:"chatName"`
	ContactName    string `json:"contactName"`
	AuthorName     string `json:"authorName"`
	Username       string `json:"username"`
	CreatedAt      string `json:"createdAt"`
	DateTime       string `json:"dateTime"`
	Timestamp      string `json:"timestamp"`
	IsIncoming     *bool  `json:"isIncoming"`
	FromMe         *bool  `json:"fromMe"`
	IsEcho         *bool  `json:"isEcho"`
	Direction      string `json:"direction"`
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
	if authHeader == "" {
		return false
	}
	expectedHash = strings.TrimSpace(expectedHash)
	// Try "Bearer <key>" format first
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		plain := strings.TrimSpace(authHeader[7:])
		sum := sha256.Sum256([]byte(plain))
		if hex.EncodeToString(sum[:]) == expectedHash {
			return true
		}
	}
	// Fallback: try the raw header value as the key (some integrations omit Bearer)
	sum := sha256.Sum256([]byte(authHeader))
	return hex.EncodeToString(sum[:]) == expectedHash
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func isSupportedTransport(transport string) bool {
	switch normalizeTransport(transport) {
	case "whatsapp", "telegram", "instagram":
		return true
	default:
		return false
	}
}

func isActiveChannelStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "connected", "enabled", "ok", "online", "working":
		return true
	default:
		return false
	}
}

func parseWebhookTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now()
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	if unix, err := parseUnixTimestamp(value); err == nil {
		return unix
	}
	return time.Now()
}

func parseUnixTimestamp(value string) (time.Time, error) {
	var n int64
	for _, r := range value {
		if r < '0' || r > '9' {
			return time.Time{}, fmt.Errorf("not unix timestamp")
		}
		n = n*10 + int64(r-'0')
	}
	if n > 1_000_000_000_000 {
		return time.UnixMilli(n), nil
	}
	return time.Unix(n, 0), nil
}

func isOutgoing(m webhookMessage) bool {
	if m.FromMe != nil {
		return *m.FromMe
	}
	if m.IsEcho != nil {
		return *m.IsEcho
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
