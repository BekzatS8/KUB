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

	"turcompany/internal/authz"
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
	// ErrChannelDeleteUnsupported — провайдер не умеет удалять канал по API.
	// В User API v3 метода удаления нет вовсе (только GET /v3/channels), канал
	// отключают руками в кабинете Wazzup. В Tech Partner API удаление есть.
	ErrChannelDeleteUnsupported = errors.New("wazzup channel delete is not supported by this driver")
	// ErrUserRolesUnsupported — драйвер не умеет выдавать роли на каналах.
	// В User API v3 это делается руками в кабинете Wazzup.
	ErrUserRolesUnsupported = errors.New("wazzup user roles are not supported by this driver")
)

type Service struct {
	repo               repositories.WazzupRepository
	defaultChannelID   string
	webhookVerifyToken string
	webhookBaseURL     string
	// accounts — подключённые аккаунты Wazzup (см. accounts.go). Первый —
	// аккаунт по умолчанию.
	accounts []*AccountConfig
}

type SetupResponse struct {
	Account      string `json:"account"`
	WebhookURL   string `json:"webhook_url"`
	WebhookToken string `json:"webhook_token"`
	CRMKey       string `json:"crm_key"`
}

type IframeOptions struct {
	// Account — чей мессенджер открыть (main | child). Для перехода на
	// конкретную переписку аккаунт берётся по каналу этой переписки.
	Account   string `json:"account,omitempty"`
	Transport string `json:"transport,omitempty"`
	ChannelID string `json:"channel_id,omitempty"`
	// ChatID — открыть iframe сразу на этой переписке (deep-link из карточки
	// клиента/лида). Для WhatsApp — номер телефона (цифры), для Telegram/
	// Instagram — username.
	ChatID string `json:"chat_id,omitempty"`
}

type IframeResponse struct {
	// Account — аккаунт, чей мессенджер открыт. Может отличаться от
	// запрошенного, если переписка принадлежит другому аккаунту.
	Account         string `json:"account"`
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
	s := &Service{
		repo:               repo,
		defaultChannelID:   strings.TrimSpace(defaultChannelID),
		webhookVerifyToken: strings.TrimSpace(webhookVerifyToken),
		webhookBaseURL:     strings.TrimSpace(webhookBaseURL),
	}
	// Основной аккаунт — из параметров конструктора; дочерний добавляется
	// через RegisterAccount. client = nil — основного аккаунта нет.
	if client != nil {
		s.RegisterAccount(AccountConfig{Name: AccountMain, Title: "Основной аккаунт", Client: client, APIKey: defaultAPIToken})
	}
	return s
}

// Setup подключает аккаунт: создаёт (или обновляет) его подключение и
// регистрирует у провайдера вебхук на URL с токеном этого подключения.
//
// Новый crmKey сохраняется только после того, как провайдер его принял.
// Раньше хэш писался в базу до регистрации вебхука, и неудачная регистрация
// оставляла базу и Wazzup с разными ключами — входящие начинали отклоняться.
func (s *Service) Setup(ctx context.Context, ownerUserID int, account, webhooksBaseURL string, enabled bool) (*SetupResponse, error) {
	acc, err := s.accountByName(account)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(strings.TrimSpace(webhooksBaseURL), "/")
	if base == "" {
		base = strings.TrimRight(s.webhookBaseURL, "/")
	}
	if base == "" {
		return nil, fmt.Errorf("%w: webhooks base url is required", ErrBadRequest)
	}
	existing, err := s.repo.GetIntegrationByAccount(ctx, acc.Name)
	if err != nil {
		return nil, err
	}
	apiKey := s.apiKeyFor(acc, existing)
	if enabled && !acc.Partner && apiKey == "" {
		return nil, fmt.Errorf("%w: wazzup api token is required", ErrBadRequest)
	}
	crmKey, crmHash, err := generateCRMKey()
	if err != nil {
		return nil, err
	}

	// Шаг 1: получить токен подключения, не трогая действующий ключ.
	currentHash := crmHash
	if existing != nil && existing.CRMKeyHash != "" {
		currentHash = existing.CRMKeyHash
	}
	_, webhookToken, err := s.repo.UpsertIntegrationByAccount(ctx, acc.Name, ownerUserID, apiKey, currentHash, "", enabled)
	if err != nil {
		return nil, err
	}
	webhooksURI := base + "/integrations/wazzup/webhook/" + webhookToken

	// Шаг 2: зарегистрировать вебхук у провайдера.
	if enabled {
		log.Printf("integration=wazzup operation=setup account=%s owner_user_id=%d", acc.Name, ownerUserID)
		if err := acc.Client.PatchWebhooks(ctx, apiKey, webhooksURI, crmKey); err != nil {
			log.Printf("integration=wazzup operation=setup status=failed account=%s owner_user_id=%d err=%v", acc.Name, ownerUserID, err)
			return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
		}
	}

	// Шаг 3: провайдер принял ключ — теперь можно сохранить его хэш.
	if _, webhookToken, err = s.repo.UpsertIntegrationByAccount(ctx, acc.Name, ownerUserID, apiKey, crmHash, webhooksURI, enabled); err != nil {
		return nil, err
	}
	return &SetupResponse{Account: acc.Name, WebhookURL: webhooksURI, WebhookToken: webhookToken, CRMKey: crmKey}, nil
}

func (s *Service) GetIframeURL(ctx context.Context, ownerUserID int, companyID int, userName string) (string, error) {
	resp, err := s.GetIframe(ctx, ownerUserID, companyID, userName, IframeOptions{})
	if err != nil {
		return "", err
	}
	return resp.URL, nil
}

func (s *Service) GetIframe(ctx context.Context, ownerUserID int, companyID int, userName string, opts IframeOptions) (*IframeResponse, error) {
	acc, integration, err := s.iframeAccount(ctx, opts)
	if err != nil {
		return nil, err
	}
	transport := normalizeTransport(opts.Transport)
	channelID := strings.TrimSpace(opts.ChannelID)
	if transport != "" && !isSupportedTransport(transport) {
		return nil, fmt.Errorf("%w: unsupported transport", ErrBadRequest)
	}
	if channelID != "" {
		ch, err := s.resolveIframeChannel(ctx, acc, integration, transport, channelID)
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
	wazzupUserID := wazzupUserIDFor(companyID, ownerUserID)
	apiKey := s.apiKeyFor(acc, integration)
	if err := acc.Client.UpsertUsers(ctx, apiKey, []UserUpsert{{ID: wazzupUserID, Name: name}}); err != nil {
		log.Printf("integration=wazzup operation=iframe_upsert_users status=failed owner_user_id=%d err=%v", ownerUserID, err)
		return nil, fmt.Errorf("%w: %v", ErrUsersSync, ErrUpstream)
	}
	// Deep-link: если запрошен конкретный чат — открываем iframe сразу на нём
	// (карточка клиента/лида → переписка в нашем мессенджере, без ручного
	// поиска). Wazzup для activeChat требует channelId — БЕЗ него показывается
	// пустой вид (заказчик: «отправляет на ватсап, но нет информации»). Поэтому
	// резолвим канал по транспорту; если канал не найден — activeChat НЕ ставим,
	// откроется обычный рабочий инбокс (а не пустой экран).
	var activeChat *IframeActiveChat
	if chatID := strings.TrimSpace(opts.ChatID); chatID != "" && transport != "" {
		acChannelID := channelID
		// 1) Канал САМОГО чата (из нашей БД) — чтобы переписка открывалась от
		//    правильного номера, а не от «первого активного»/пустого канала.
		if acChannelID == "" {
			if chID, cerr := s.repo.GetChatChannelID(ctx, transport, chatID); cerr == nil && chID != "" {
				acChannelID = chID
			}
		}
		// 2) Fallback: первый активный канал транспорта (старое поведение).
		if acChannelID == "" {
			if ch, rerr := s.resolveIframeChannel(ctx, acc, integration, transport, ""); rerr == nil && ch != nil {
				acChannelID = strings.TrimSpace(ch.ExternalChannelID)
			}
		}
		if acChannelID != "" {
			activeChat = &IframeActiveChat{
				ChannelID: acChannelID,
				ChatType:  transport,
				ChatID:    chatID,
			}
		}
	}
	url, err := acc.Client.CreateIframe(ctx, apiKey, CreateIframeRequest{
		User:       UserUpsert{ID: wazzupUserID, Name: name},
		Scope:      "global",
		ActiveChat: activeChat,
	})
	if err != nil {
		log.Printf("integration=wazzup operation=iframe_create status=failed owner_user_id=%d err=%v", ownerUserID, err)
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	resp := &IframeResponse{
		Account:   acc.Name,
		URL:       url,
		IframeURL: url,
		Transport: transport,
		ChannelID: channelID,
	}
	if transport != "" && channelID == "" {
		resp.Message = "Wazzup returned global iframe only"
	}
	return resp, nil
}

// iframeAccount выбирает аккаунт для окна мессенджера. Переход на конкретную
// переписку (из карточки клиента или лида) открывается в аккаунте, к номеру
// которого относится эта переписка, — даже если пришли из другого пункта
// меню: в чужом аккаунте её просто нет.
func (s *Service) iframeAccount(ctx context.Context, opts IframeOptions) (*AccountConfig, *models.WazzupIntegration, error) {
	transport := normalizeTransport(opts.Transport)
	if chatID := strings.TrimSpace(opts.ChatID); chatID != "" && transport != "" {
		if chID, err := s.repo.GetChatChannelID(ctx, transport, chatID); err == nil && chID != "" {
			if acc, integration, err := s.accountOfChannel(ctx, chID); err == nil && acc != nil {
				return acc, integration, nil
			}
		}
	}
	if strings.TrimSpace(opts.Account) != "" {
		return s.connectedAccount(ctx, opts.Account)
	}
	return s.firstConnected(ctx)
}

func (s *Service) resolveIframeChannel(ctx context.Context, acc *AccountConfig, integration *models.WazzupIntegration, transport, channelID string) (*models.WazzupChannel, error) {
	channels, err := s.syncAccountChannels(ctx, acc, integration)
	if err != nil {
		cached, cacheErr := s.repo.ListChannels(ctx, integration.ID)
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
	if _, _, err := s.firstConnected(ctx); err != nil {
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

// SyncChannels обновляет справочник номеров всех подключённых аккаунтов и
// возвращает их одним списком; у каждого номера указан его аккаунт. Сбой у
// одного аккаунта не прячет номера другого.
func (s *Service) SyncChannels(ctx context.Context, ownerUserID int) ([]models.WazzupChannel, error) {
	all := make([]models.WazzupChannel, 0)
	connectedAny := false
	var lastErr error
	for _, acc := range s.accounts {
		integration, err := s.connection(ctx, acc)
		if err != nil {
			if errors.Is(err, ErrNotFound) || errors.Is(err, ErrDisabled) {
				continue
			}
			return nil, err
		}
		connectedAny = true
		channels, err := s.syncAccountChannels(ctx, acc, integration)
		if err != nil {
			log.Printf("integration=wazzup operation=channels_sync status=failed account=%s owner_user_id=%d err=%v", acc.Name, ownerUserID, err)
			lastErr = err
			continue
		}
		all = append(all, channels...)
	}
	if !connectedAny {
		return nil, ErrAccountNotConnected
	}
	if len(all) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return all, nil
}

// syncAccountChannels — синхронизация номеров одного аккаунта.
func (s *Service) syncAccountChannels(ctx context.Context, acc *AccountConfig, integration *models.WazzupIntegration) ([]models.WazzupChannel, error) {
	apiKey := s.apiKeyFor(acc, integration)
	providerChannels, err := acc.Client.ListChannels(ctx, apiKey)
	if err != nil {
		log.Printf("integration=wazzup operation=channels_sync status=failed account=%s err=%v", acc.Name, err)
		cached, cacheErr := s.repo.ListChannels(ctx, integration.ID)
		if cacheErr == nil && len(cached) > 0 {
			return withAccount(cached, acc.Name), nil
		}
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}

	// Номер, удалённый вручную, Wazzup может продолжать отдавать (так ведёт
	// себя заблокированный Meta номер): пропускаем его, пока он не работает.
	// Заработал снова — например, переподключили в кабинете — возвращаем.
	hidden := map[string]bool{}
	if ids, hiddenErr := s.repo.ListHiddenChannelIDs(ctx, integration.ID); hiddenErr != nil {
		log.Printf("integration=wazzup operation=channels_hidden status=failed integration_id=%d err=%v", integration.ID, hiddenErr)
	} else {
		for _, id := range ids {
			hidden[id] = true
		}
	}

	channels := make([]models.WazzupChannel, 0, len(providerChannels))
	for _, ch := range providerChannels {
		externalID := strings.TrimSpace(ch.ID)
		if hidden[externalID] {
			if !isActiveChannelStatus(ch.Status) {
				continue
			}
			if err := s.repo.UnhideChannel(ctx, integration.ID, externalID); err != nil {
				log.Printf("integration=wazzup operation=channel_unhide status=failed integration_id=%d channel=%s err=%v", integration.ID, externalID, err)
			} else {
				log.Printf("integration=wazzup operation=channel_unhide status=ok integration_id=%d channel=%s", integration.ID, externalID)
			}
		}
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
	// Каналы, которых у провайдера больше нет, удаляем: раньше синхронизация
	// только добавляла, и отключённые номера висели в справочнике навсегда —
	// попадали в выбор «Написать первым» и в привязку к филиалам.
	// Пустой ответ провайдера сюда не доходит (см. проверку в репозитории),
	// поэтому случайно вычистить весь справочник нельзя.
	keep := make([]string, 0, len(channels))
	for _, ch := range channels {
		keep = append(keep, ch.ExternalChannelID)
	}
	if removed, pruneErr := s.repo.DeleteChannelsNotIn(ctx, integration.ID, keep); pruneErr != nil {
		// Не роняем синхронизацию из-за уборки — список всё равно вернём.
		log.Printf("integration=wazzup operation=channels_prune status=failed integration_id=%d err=%v", integration.ID, pruneErr)
	} else if removed > 0 {
		log.Printf("integration=wazzup operation=channels_prune status=ok integration_id=%d removed=%d", integration.ID, removed)
	}

	stored, err := s.repo.ListChannels(ctx, integration.ID)
	if err != nil {
		return nil, err
	}
	// Роли на каналах: без них сотрудник видит в мессенджере «Нет доступа к
	// чатам». Новый канал приходит без ролей, поэтому раздаём их здесь же —
	// на каждой синхронизации, а не однократно при настройке.
	s.syncUserRoles(ctx, acc, apiKey, stored)
	return withAccount(stored, acc.Name), nil
}

// withAccount помечает номера их аккаунтом — интерфейс группирует по нему.
func withAccount(channels []models.WazzupChannel, account string) []models.WazzupChannel {
	for i := range channels {
		channels[i].Account = account
	}
	return channels
}

func (s *Service) ListDialogs(ctx context.Context, userID int, transport string) ([]models.WazzupDialog, error) {
	if _, _, err := s.firstConnected(ctx); err != nil {
		return nil, err
	}
	return s.repo.ListExternalDialogs(ctx, userID, normalizeTransport(transport))
}

func (s *Service) ListDialogMessages(ctx context.Context, userID, dialogID, limit, offset int) ([]models.WazzupDialogMessage, error) {
	if _, _, err := s.firstConnected(ctx); err != nil {
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
	// Сначала пробуем формат Tech Partner API (v2) — он приходит конвертом
	// {"event":…,"data":[…]}. Если это не он, разбираем как User API v3.
	var req webhookPayload
	partnerMessages, isPartner := parsePartnerWebhook(payload)
	if isPartner {
		req.Messages = partnerMessages
	} else if err := json.Unmarshal(payload, &req); err != nil {
		return 0, false, ErrBadPayload
	}
	// Wazzup may not send Authorization header — rely on webhook token for auth.
	// If header IS present, validate it as an extra security check.
	//
	// В v2 секрета нет вовсе: подписка задаётся одним лишь URL, поэтому
	// проверять crmKey нечем и защитой служит неугадываемый токен в пути.
	if !isPartner && strings.TrimSpace(authHeader) != "" && !validateCRMKey(authHeader, integration.CRMKeyHash) {
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

	// Филиал входящего лида определяется каналом Wazzup (Алматы/Шымкент = разные
	// филиалы). Если канал не привязан к филиалу — nil, лид получит филиал
	// владельца интеграции (fallback в CreateLeadFromInbound).
	var inboundBranch *int
	if b, berr := s.repo.GetChannelBranchID(ctx, integration.ID, channelID); berr == nil {
		inboundBranch = b
	}

	// Отдел входящего лида — тоже по каналу. Выделенная линия отдела (например
	// номер жалоб и претензий ОКК) не должна попадать в общий пул лидов
	// филиалов (обратная связь заказчика 18.09.2026).
	var inboundDepartment *int
	if d, derr := s.repo.GetChannelDepartmentID(ctx, integration.ID, channelID); derr == nil {
		inboundDepartment = d
	}

	var clientIDPtr *int
	var leadIDPtr *int
	phone := ""

	switch transport {
	case "whatsapp":
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
				leadID, err = s.repo.CreateLeadFromInbound(ctx, integration.OwnerUserID, inboundBranch, inboundDepartment, phone, "whatsapp", text)
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

	case "telegram", "instagram":
		// For Telegram/Instagram we identify contacts by externalChatID (not phone).
		// First try to find an existing lead already linked to this chat.
		leadID, err = s.repo.FindLeadByExternalChatID(ctx, transport, externalChatID)
		if err != nil {
			return 0, false, false, err
		}
		if leadID == 0 {
			// Also check by phone if available (e.g. Telegram sometimes sends phone).
			phoneCandidate := normalizePhone(firstNonEmpty(m.Username))
			if phoneCandidate == "" {
				phoneCandidate = normalizePhone(externalChatID)
			}
			if phoneCandidate != "" && len(phoneCandidate) >= 7 {
				phone = phoneCandidate
				cid, err := s.repo.FindClientByPhone(ctx, phone)
				if err != nil {
					return 0, false, false, err
				}
				if cid > 0 {
					clientIDPtr = &cid
				}
				leadID, err = s.repo.FindLeadByPhone(ctx, phone)
				if err != nil {
					return 0, false, false, err
				}
			}
		}
		if leadID == 0 {
			leadID, err = s.repo.CreateLeadFromInbound(ctx, integration.OwnerUserID, inboundBranch, inboundDepartment, phone, transport, text)
			if err != nil {
				return 0, false, false, err
			}
			leadCreated = true
		} else {
			if err := s.repo.UpdateLeadDescriptionIfEmpty(ctx, leadID, text); err != nil {
				return 0, false, false, err
			}
		}
		if leadID > 0 {
			leadIDPtr = &leadID
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

func (s *Service) SendMessage(ctx context.Context, ownerUserID int, chatID, transport, channelID, text string) (*SendMessageResponse, error) {
	// Писать нужно через аккаунт того номера, с которого отправляем: чужой
	// аккаунт о таком канале не знает и отправку отклонит.
	acc, integration, err := s.accountOfChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		if acc, integration, err = s.firstConnected(ctx); err != nil {
			return nil, err
		}
	}
	transport = normalizeTransport(transport)
	if transport == "" {
		transport = "whatsapp" // основной сценарий «написать первым» — WhatsApp
	}
	// channelId обязателен. Если фронт указал канал явно (выбор филиала «с какого
	// номера писать») — используем его; иначе подбираем канал нужного транспорта.
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		channelID = s.resolveSendChannel(ctx, integration.ID, transport)
	}
	apiKey := s.apiKeyFor(acc, integration)
	req := SendMessageRequest{
		ChannelID: channelID,
		ChatType:  transport,
		ChatID:    strings.TrimSpace(chatID),
		Text:      strings.TrimSpace(text),
		// Автор — тот, кто реально пишет из CRM, а не владелец интеграции.
		CRMUserID: s.ensureWazzupUser(ctx, acc, apiKey, ownerUserID),
	}
	resp, err := s.sendWithAuthor(ctx, acc, apiKey, req)
	if err != nil {
		log.Printf("integration=wazzup operation=send_message status=failed owner_user_id=%d transport=%s target_chat=%s err=%v", ownerUserID, transport, maskChatID(chatID), err)
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	log.Printf("integration=wazzup operation=send_message status=ok owner_user_id=%d transport=%s target_chat=%s message_id=%s", ownerUserID, transport, maskChatID(chatID), tokenPrefix(resp.MessageID))
	return resp, nil
}

// wazzupUserIDFor — id сотрудника в терминах Wazzup. Тот же формат, что уже
// использует iframe, чтобы один и тот же человек не задваивался: сообщения,
// отправленные из нашего интерфейса, и переписка в iframe принадлежат одному
// пользователю Wazzup.
func wazzupUserIDFor(companyID, userID int) string {
	if userID <= 0 {
		return ""
	}
	if companyID <= 0 {
		companyID = userID
	}
	return fmt.Sprintf("kub-%d-%d", companyID, userID)
}

// ensureWazzupUser регистрирует автора сообщения в Wazzup и возвращает его id
// для поля crmUserId. Без этого Wazzup считает автором саму интеграцию и
// подписывает сообщение «API • Admin», хотя писал менеджер отдела (обратная
// связь заказчика 18.09.2026).
//
// Ошибку синхронизации не пробрасываем: подпись автора не повод не отправить
// сообщение клиенту — вернём пустой id и уйдём прежним путём.
func (s *Service) ensureWazzupUser(ctx context.Context, acc *AccountConfig, apiKey string, userID int) string {
	id := wazzupUserIDFor(0, userID)
	if id == "" {
		return ""
	}
	name := ""
	if crmUser, err := s.repo.GetCRMUserByID(ctx, userID); err == nil && crmUser != nil {
		name = strings.TrimSpace(crmUser.Name)
	}
	if name == "" {
		name = fmt.Sprintf("User %d", userID)
	}
	if err := acc.Client.UpsertUsers(ctx, apiKey, []UserUpsert{{ID: id, Name: name}}); err != nil {
		log.Printf("integration=wazzup operation=send_upsert_user status=failed user_id=%d err=%v", userID, err)
		return ""
	}
	return id
}

// sendWithAuthor отправляет сообщение от имени сотрудника и, если провайдер не
// принял автора, повторяет отправку без crmUserId. Клиент должен получить
// сообщение даже тогда, когда Wazzup не знает такого пользователя.
func (s *Service) sendWithAuthor(ctx context.Context, acc *AccountConfig, apiKey string, req SendMessageRequest) (*SendMessageResponse, error) {
	resp, err := acc.Client.SendMessage(ctx, apiKey, req)
	if err == nil || req.CRMUserID == "" {
		return resp, err
	}
	if !mentionsCRMUser(err) {
		return resp, err
	}
	log.Printf("integration=wazzup operation=send_message status=retry_without_author err=%v", err)
	req.CRMUserID = ""
	return acc.Client.SendMessage(ctx, apiKey, req)
}

// mentionsCRMUser — провайдер отказал именно из-за автора сообщения.
func mentionsCRMUser(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "crmuser") || strings.Contains(text, "crm_user") || strings.Contains(text, "user not found")
}

// resolveSendChannel возвращает externalChannelID канала нужного транспорта.
// Порядок: канал транспорта из БД → defaultChannelID из конфига.
func (s *Service) resolveSendChannel(ctx context.Context, integrationID int, transport string) string {
	channels, err := s.repo.ListChannels(ctx, integrationID)
	if err == nil {
		for _, ch := range channels {
			if normalizeTransport(ch.Transport) == transport && strings.TrimSpace(ch.ExternalChannelID) != "" {
				return strings.TrimSpace(ch.ExternalChannelID)
			}
		}
	}
	return s.defaultChannelID
}

func (s *Service) SendDialogMessage(ctx context.Context, userID, dialogID int, text string) (*models.WazzupDialogMessage, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("%w: text is required", ErrBadRequest)
	}
	dialog, err := s.repo.GetExternalDialog(ctx, userID, dialogID)
	if err != nil {
		return nil, err
	}
	if dialog == nil {
		return nil, ErrNotFound
	}
	// Ответ уходит через аккаунт номера, на который писал клиент.
	acc, integration, err := s.accountOfChannel(ctx, dialog.ExternalChannelID)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		if acc, integration, err = s.firstConnected(ctx); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(dialog.ExternalChatID) == "" {
		return nil, fmt.Errorf("%w: external chat id is required", ErrBadRequest)
	}
	channelID := strings.TrimSpace(firstNonEmpty(dialog.ExternalChannelID, s.defaultChannelID))
	if channelID == "" {
		return nil, fmt.Errorf("%w: channel id is required", ErrBadRequest)
	}

	apiKey := s.apiKeyFor(acc, integration)
	resp, err := s.sendWithAuthor(ctx, acc, apiKey, SendMessageRequest{
		ChannelID: channelID,
		ChatType:  normalizeTransport(dialog.Transport),
		ChatID:    dialog.ExternalChatID,
		Text:      text,
		CRMUserID: s.ensureWazzupUser(ctx, acc, apiKey, userID),
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
	// "outbound" — формат Tech Partner API (v2). Без него исходящие сообщения
	// считались бы входящими и плодили лиды на собственные ответы менеджеров.
	return d == "out" || d == "outgoing" || d == "outbound"
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

// DeleteChannel удаляет канал: сначала у провайдера, затем строку в CRM.
//
// Раньше удалялась только строка в базе, и на ближайшей синхронизации канал
// возвращался из ответа провайдера — кнопка выглядела сломанной. Для White
// Label это особенно важно: кабинета у дочернего аккаунта нет, и CRM остаётся
// единственной точкой управления каналами.
//
// Второй результат говорит, удалось ли удалить канал у провайдера. На драйвере
// v3 удаления в API нет (ErrChannelDeleteUnsupported) — тогда чистим только
// строку в CRM, как и прежде, а канал нужно отключить в кабинете Wazzup.
func (s *Service) DeleteChannel(ctx context.Context, ownerUserID int, channelID int64) (providerDeleted bool, err error) {
	acc, integration, channel, err := s.accountOfChannelRow(ctx, channelID)
	if err != nil {
		return false, err
	}
	externalID := ""
	if channel != nil {
		externalID = strings.TrimSpace(channel.ExternalChannelID)
	}
	if acc == nil || externalID == "" {
		// Канала нет среди актуальных: строка осталась от прежней интеграции —
		// просто убираем её.
		return false, s.repo.DeleteChannel(ctx, channelID)
	}

	apiKey := s.apiKeyFor(acc, integration)
	// delete_chats=false: переписку сохраняем, удаляем только сам канал.
	switch err := acc.Client.DeleteChannel(ctx, apiKey, externalID, false); {
	case err == nil:
		providerDeleted = true
	case errors.Is(err, ErrChannelDeleteUnsupported):
		providerDeleted = false
	default:
		// Не удалось удалить у провайдера — строку в CRM не трогаем, иначе
		// канал вернётся на следующей синхронизации и это будет выглядеть как
		// «удаление не работает».
		log.Printf("integration=wazzup operation=channel_delete status=failed integration_id=%d channel=%s err=%v", integration.ID, externalID, err)
		return false, fmt.Errorf("%w: %v", ErrUpstream, err)
	}

	// Запоминаем удаление: иначе номер, который Wazzup продолжает отдавать,
	// вернётся на ближайшей синхронизации.
	if err := s.repo.HideChannel(ctx, integration.ID, externalID); err != nil {
		return providerDeleted, err
	}
	if err := s.repo.DeleteChannel(ctx, channelID); err != nil {
		return providerDeleted, err
	}
	log.Printf("integration=wazzup operation=channel_delete status=ok integration_id=%d channel=%s provider_deleted=%v", integration.ID, externalID, providerDeleted)
	return providerDeleted, nil
}

// wazzupRoleFor переводит роль CRM в роль Wazzup на канале.
//
//	seller  — пишет только своим контактам, чужих переписок не видит;
//	manager — видит все чаты и может писать кому угодно;
//	auditor — видит все чаты, но отправка запрещена.
//
// Отображение повторяет ролевую модель CRM: ОКК — наблюдатель без права
// писать, руководство и админ видят всё, остальные бизнес-роли ведут своих
// клиентов.
func wazzupRoleFor(roleID int) string {
	switch roleID {
	case authz.RoleControl:
		return "auditor"
	case authz.RoleManagement, authz.RoleSystemAdmin:
		return "manager"
	default:
		return "seller"
	}
}

// syncUserRoles выдаёт всем активным сотрудникам роли на всех каналах.
//
// Без роли сотрудник открывает мессенджер и видит «Нет доступа к чатам». В
// обычном аккаунте роли раздаются в кабинете Wazzup, но у дочернего White Label
// аккаунта кабинета нет — значит это обязанность CRM.
//
// Вызывается после синхронизации каналов и работает по принципу «лучшее
// усилие»: ошибка логируется, но не роняет выдачу списка каналов.
func (s *Service) syncUserRoles(ctx context.Context, acc *AccountConfig, apiKey string, channels []models.WazzupChannel) {
	if len(channels) == 0 {
		return
	}
	users, err := s.repo.ListCRMUsers(ctx)
	if err != nil {
		log.Printf("integration=wazzup operation=user_roles status=failed reason=list_users err=%v", err)
		return
	}

	roles := make([]UserChannelRole, 0, len(users)*len(channels))
	for _, u := range users {
		wazzupUserID := wazzupUserIDFor(u.ID, u.ID)
		if wazzupUserID == "" {
			continue
		}
		role := wazzupRoleFor(u.RoleID)
		for _, ch := range channels {
			externalID := strings.TrimSpace(ch.ExternalChannelID)
			if externalID == "" {
				continue
			}
			roles = append(roles, UserChannelRole{
				ChannelID: externalID,
				UserID:    wazzupUserID,
				Role:      role,
				// Обращения от новых контактов разбирают те, кто ведёт клиентов.
				// Руководство и ОКК в очередь на новых клиентов не встают.
				AllowGetNewClients: role == "seller",
			})
		}
	}
	if len(roles) == 0 {
		return
	}

	switch err := acc.Client.SyncUserRoles(ctx, apiKey, roles); {
	case err == nil:
		log.Printf("integration=wazzup operation=user_roles status=ok users=%d channels=%d", len(users), len(channels))
	case errors.Is(err, ErrUserRolesUnsupported):
		// Драйвер v3: роли раздаются в кабинете Wazzup, это нормально.
	default:
		log.Printf("integration=wazzup operation=user_roles status=failed err=%v", err)
	}
}
