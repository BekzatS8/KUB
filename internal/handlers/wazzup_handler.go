package handlers

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/authz"
	wz "turcompany/internal/integrations/wazzup"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type WazzupService interface {
	Setup(ctx context.Context, ownerUserID int, account, webhooksBaseURL string, enabled bool) (*wz.SetupResponse, error)
	Accounts(ctx context.Context) ([]wz.AccountInfo, error)
	GetIframeURL(ctx context.Context, ownerUserID int, companyID int, userName string) (string, error)
	GetIframe(ctx context.Context, ownerUserID int, companyID int, userName string, opts wz.IframeOptions) (*wz.IframeResponse, error)
	GetStatus(ctx context.Context, ownerUserID int) (*models.WazzupStatus, error)
	SyncChannels(ctx context.Context, ownerUserID int) ([]models.WazzupChannel, error)
	ListDialogs(ctx context.Context, userID int, transport string) ([]models.WazzupDialog, error)
	ListDialogMessages(ctx context.Context, userID, dialogID, limit, offset int) ([]models.WazzupDialogMessage, error)
	HandleWebhook(ctx context.Context, token string, authHeader string, payload []byte) (leadID int, created bool, err error)
	SendMessage(ctx context.Context, ownerUserID int, chatID, transport, channelID, text string) (*wz.SendMessageResponse, error)
	SendDialogMessage(ctx context.Context, userID, dialogID int, text string) (*models.WazzupDialogMessage, error)
	DeleteChannel(ctx context.Context, ownerUserID int, channelID int64) (providerDeleted bool, err error)
}

type WazzupHandler struct {
	svc  WazzupService
	repo repositories.WazzupRepository
	wl   *wz.WhiteLabelClient // White Label: встроенный iframe добавления каналов (может быть nil)
}

func NewWazzupHandler(svc WazzupService) *WazzupHandler {
	return &WazzupHandler{svc: svc}
}

func NewWazzupHandlerWithRepo(svc WazzupService, repo repositories.WazzupRepository) *WazzupHandler {
	return &WazzupHandler{svc: svc, repo: repo}
}

// SetWhiteLabel подключает White Label клиент (встроенное добавление каналов).
func (h *WazzupHandler) SetWhiteLabel(wl *wz.WhiteLabelClient) {
	h.wl = wl
}

type wazzupSetupRequest struct {
	// Account — какой аккаунт подключить: main (основной) или child
	// (дочерний). Пусто — основной, как раньше.
	Account         string `json:"account"`
	WebhooksBaseURL string `json:"webhooks_base_url"`
	Enabled         bool   `json:"enabled"`
}

type wazzupIframeRequest struct {
	// Account — чей мессенджер открыть (main | child).
	Account   string `json:"account"`
	Transport string `json:"transport"`
	ChannelID string `json:"channel_id"`
	// ChatID — открыть iframe сразу на этой переписке (deep-link). Для WhatsApp
	// это номер телефона (цифры), для Telegram/Instagram — username.
	ChatID string `json:"chat_id"`
}

type wazzupSendMessageRequest struct {
	ChatID    string `json:"chat_id"`
	Transport string `json:"transport"`
	ChannelID string `json:"channel_id"`
	Text      string `json:"text"`
}

type wazzupDialogSendRequest struct {
	Text string `json:"text"`
}

func (h *WazzupHandler) Webhook(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		badRequest(c, "invalid body")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	leadID, created, err := h.svc.HandleWebhook(ctx, token, c.GetHeader("Authorization"), body)
	if err != nil {
		payloadPreview := string(body)
		if len(payloadPreview) > 300 {
			payloadPreview = payloadPreview[:300]
		}
		log.Printf("[WAZZUP][webhook] err=%v payload_prefix=%q", err, payloadPreview)
		switch {
		case errors.Is(err, wz.ErrUnauthorized):
			unauthorized(c, "invalid authorization")
		case errors.Is(err, wz.ErrNotFound), errors.Is(err, wz.ErrDisabled):
			notFound(c, "wazzup_integration_not_found", "Integration not found")
		case errors.Is(err, wz.ErrBadPayload):
			badRequest(c, "invalid payload")
		default:
			internalError(c, "failed to process webhook")
		}
		return
	}
	resp := gin.H{"status": "ok"}
	if created {
		resp["lead_id"] = leadID
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WazzupHandler) Setup(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !authz.CanManageIntegrations(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	var req wazzupSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	resp, err := h.svc.Setup(ctx, userID, req.Account, req.WebhooksBaseURL, req.Enabled)
	if err != nil {
		log.Printf("[WAZZUP][setup] user_id=%d base_url=%q enabled=%v err=%v", userID, req.WebhooksBaseURL, req.Enabled, err)
		if errors.Is(err, wz.ErrUpstream) {
			log.Printf("[WAZZUP][setup] upstream_error=%v", err)
		}
		switch {
		case errors.Is(err, wz.ErrAccountNotConfigured):
			notFound(c, "wazzup_account_not_configured", "Аккаунт Wazzup не настроен на сервере")
		case errors.Is(err, wz.ErrBadRequest):
			badRequest(c, err.Error())
		case errors.Is(err, wz.ErrUpstream):
			c.JSON(http.StatusBadGateway, gin.H{"error": "wazzup upstream error"})
		default:
			internalError(c, "failed to setup wazzup")
		}
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WazzupHandler) SendMessage(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	var req wazzupSendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	resp, err := h.svc.SendMessage(ctx, userID, req.ChatID, req.Transport, req.ChannelID, req.Text)
	if err != nil {
		switch {
		case errors.Is(err, wz.ErrDisabled), errors.Is(err, wz.ErrNotFound):
			notFound(c, "wazzup_integration_not_found", "Integration not found")
		case errors.Is(err, wz.ErrUpstream):
			c.JSON(http.StatusBadGateway, gin.H{"error": "wazzup upstream error"})
		default:
			internalError(c, "failed to send wazzup message")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "message_id": resp.MessageID})
}

func (h *WazzupHandler) Iframe(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	companyID, _ := getIntFromCtx(c, "company_id")
	if companyID == 0 {
		companyID, _ = getIntFromCtx(c, "tenant_id")
	}
	var req wazzupIframeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()

	userName := ""
	if h.repo != nil {
		crmUser, repoErr := h.repo.GetCRMUserByID(ctx, userID)
		if repoErr != nil {
			internalError(c, "failed to resolve user")
			return
		}
		if crmUser != nil {
			userName = crmUser.Name
		}
	}

	resp, err := h.svc.GetIframe(ctx, userID, companyID, userName, wz.IframeOptions{
		Account:   req.Account,
		Transport: req.Transport,
		ChannelID: req.ChannelID,
		ChatID:    req.ChatID,
	})
	if err != nil {
		log.Printf("[WAZZUP][iframe] user_id=%d err=%v", userID, err)
		switch {
		case errors.Is(err, wz.ErrBadRequest):
			badRequest(c, err.Error())
		case errors.Is(err, wz.ErrUsersSync):
			c.JSON(http.StatusBadGateway, gin.H{"error": "Wazzup users sync failed"})
		case errors.Is(err, wz.ErrNotFound), errors.Is(err, wz.ErrDisabled):
			notFound(c, "wazzup_integration_not_found", "Integration not found")
		case errors.Is(err, wz.ErrUpstream):
			c.JSON(http.StatusBadGateway, gin.H{"error": "wazzup upstream error"})
		default:
			internalError(c, "failed to get iframe")
		}
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WazzupHandler) Status(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	status, err := h.svc.GetStatus(ctx, userID)
	if err != nil {
		writeWazzupError(c, err, "failed to get wazzup status")
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *WazzupHandler) Channels(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	channels, err := h.svc.SyncChannels(ctx, userID)
	if err != nil {
		writeWazzupError(c, err, "failed to list wazzup channels")
		return
	}
	c.JSON(http.StatusOK, gin.H{"value": channels, "count": len(channels)})
}

type setChannelBranchRequest struct {
	BranchID *int `json:"branch_id"`
}

// SetChannelBranch привязывает канал Wazzup к филиалу (branch_id=null снимает
// привязку). Входящий лид из этого канала будет попадать в указанный филиал.
// Только админ/руководство — это конфигурация разделения по филиалам.
func (h *WazzupHandler) SetChannelBranch(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if roleID != authz.RoleSystemAdmin && roleID != authz.RoleManagement {
		forbidden(c, "Forbidden")
		return
	}
	if h.repo == nil {
		internalError(c, "channel branch mapping unavailable")
		return
	}
	channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || channelID <= 0 {
		badRequest(c, "Invalid channel id")
		return
	}
	var req setChannelBranchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	if req.BranchID != nil && *req.BranchID <= 0 {
		badRequest(c, "Invalid branch_id")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	if err := h.repo.SetChannelBranch(ctx, channelID, req.BranchID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			notFound(c, NotFoundCode, "Channel not found")
			return
		}
		internalError(c, "Failed to set channel branch")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type setChannelDepartmentRequest struct {
	DepartmentID *int `json:"department_id"`
}

// SetChannelDepartment привязывает канал к отделу (department_id=null снимает
// привязку). Входящие с этого канала становятся лидами этого отдела — нужно для
// выделенных линий вроде номера жалоб и претензий отдела контроля качества.
// Только админ/руководство, как и привязка к филиалу.
func (h *WazzupHandler) SetChannelDepartment(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if roleID != authz.RoleSystemAdmin && roleID != authz.RoleManagement {
		forbidden(c, "Forbidden")
		return
	}
	if h.repo == nil {
		internalError(c, "channel department mapping unavailable")
		return
	}
	channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || channelID <= 0 {
		badRequest(c, "Invalid channel id")
		return
	}
	var req setChannelDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	if req.DepartmentID != nil && *req.DepartmentID <= 0 {
		badRequest(c, "Invalid department_id")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	if err := h.repo.SetChannelDepartment(ctx, channelID, req.DepartmentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			notFound(c, NotFoundCode, "Channel not found")
			return
		}
		internalError(c, "Failed to set channel department")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Departments — справочник отделов для выбора в настройках каналов.
func (h *WazzupHandler) Departments(c *gin.Context) {
	if h.repo == nil {
		internalError(c, "departments repository is not configured")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	items, err := h.repo.ListDepartments(ctx)
	if err != nil {
		internalError(c, "Failed to list departments")
		return
	}
	c.JSON(http.StatusOK, gin.H{"value": items, "count": len(items)})
}

// DeleteChannel убирает канал из справочника CRM.
// DELETE /integrations/wazzup/channels/:id — админ/руководство.
//
// Нужен для «мусорных» строк: канал отключили на стороне Wazzup, а в CRM он
// остался. Переписка не страдает — удаляется только запись справочника вместе
// с привязкой к филиалу. Если канал ещё жив у провайдера, он вернётся при
// следующей синхронизации (уже без филиала).
func (h *WazzupHandler) DeleteChannel(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if roleID != authz.RoleSystemAdmin && roleID != authz.RoleManagement {
		forbidden(c, "Forbidden")
		return
	}
	channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || channelID <= 0 {
		badRequest(c, "Invalid channel id")
		return
	}
	userID, _ := getUserAndRole(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	// Удаление идёт через сервис: сначала у провайдера, затем строка в CRM.
	// Иначе канал возвращался на ближайшей синхронизации (см. Service.DeleteChannel).
	providerDeleted, err := h.svc.DeleteChannel(ctx, userID, channelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			notFound(c, NotFoundCode, "Channel not found")
			return
		}
		writeWazzupError(c, err, "failed to delete wazzup channel")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "provider_deleted": providerDeleted})
}

// GET /integrations/wazzup/channels/connect-link[?transport=...]
//
// Возвращает ссылку на встроенный iframe Wazzup для подключения канала (White
// Label). Фронт вставляет её в <iframe>. Только админ/руководство. Если White
// Label не настроен — 404 с кодом, фронт откатывается на кабинет Wazzup.
func (h *WazzupHandler) ChannelConnectLink(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if roleID != authz.RoleSystemAdmin && roleID != authz.RoleManagement {
		forbidden(c, "Forbidden")
		return
	}
	if h.wl == nil || !h.wl.Configured() {
		notFound(c, "wazzup_white_label_not_configured", "White Label не настроен")
		return
	}
	transport := strings.ToLower(strings.TrimSpace(c.Query("transport")))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	link, err := h.wl.ChannelsIframeLink(ctx, transport)
	if err != nil {
		log.Printf("integration=wazzup operation=channel_connect_link status=failed transport=%s err=%v", transport, err)
		// detail отдаём в ответ (эндпоинт только для админа/руководства) — чтобы
		// быстро диагностировать ошибки White Label (OAUTH_*, неверный account_id).
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "wazzup_white_label_error",
			"message": "Не удалось получить ссылку на добавление канала",
			"detail":  err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"link": link})
}

func (h *WazzupHandler) Dialogs(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	transport := strings.TrimSpace(c.Query("transport"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()

	dialogs, err := h.svc.ListDialogs(ctx, userID, transport)
	if err != nil {
		writeWazzupError(c, err, "failed to list wazzup dialogs")
		return
	}
	c.JSON(http.StatusOK, gin.H{"value": dialogs, "count": len(dialogs)})
}

func (h *WazzupHandler) DialogMessages(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	dialogID, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || dialogID <= 0 {
		badRequest(c, "Invalid dialog id")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()

	messages, err := h.svc.ListDialogMessages(ctx, userID, dialogID, limit, offset)
	if err != nil {
		writeWazzupError(c, err, "failed to list wazzup messages")
		return
	}
	c.JSON(http.StatusOK, gin.H{"value": messages, "count": len(messages)})
}

func (h *WazzupHandler) SendDialogMessage(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	dialogID, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || dialogID <= 0 {
		badRequest(c, "Invalid dialog id")
		return
	}
	var req wazzupDialogSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	msg, err := h.svc.SendDialogMessage(ctx, userID, dialogID, req.Text)
	if err != nil {
		writeWazzupError(c, err, "failed to send wazzup dialog message")
		return
	}
	c.JSON(http.StatusCreated, msg)
}

func (h *WazzupHandler) CRMUsers(c *gin.Context) {
	if h.repo == nil {
		internalError(c, "crm users repository is not configured")
		return
	}
	token := strings.TrimSpace(c.Param("token"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	integration, err := h.repo.GetIntegrationByToken(ctx, token)
	if err != nil {
		internalError(c, "failed to resolve integration")
		return
	}
	if integration == nil {
		notFound(c, "wazzup_integration_not_found", "Integration not found")
		return
	}

	users, err := h.repo.ListCRMUsers(ctx)
	if err != nil {
		internalError(c, "failed to list users")
		return
	}
	log.Printf("[WAZZUP][crm-users] count=%d", len(users))

	out := make([]gin.H, 0, len(users))
	for _, u := range users {
		out = append(out, gin.H{
			"id":     strconv.Itoa(u.ID),
			"name":   u.Name,
			"email":  u.Email,
			"phone":  u.Phone,
			"active": u.Active,
		})
	}
	c.JSON(http.StatusOK, gin.H{"users": out})
}

func (h *WazzupHandler) CRMUserByID(c *gin.Context) {
	if h.repo == nil {
		internalError(c, "crm users repository is not configured")
		return
	}
	token := strings.TrimSpace(c.Param("token"))
	userID, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || userID <= 0 {
		notFound(c, "user_not_found", "User not found")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	integration, err := h.repo.GetIntegrationByToken(ctx, token)
	if err != nil {
		internalError(c, "failed to resolve integration")
		return
	}
	if integration == nil {
		notFound(c, "wazzup_integration_not_found", "Integration not found")
		return
	}

	u, err := h.repo.GetCRMUserByID(ctx, userID)
	if err != nil {
		internalError(c, "failed to get user")
		return
	}
	if u == nil {
		notFound(c, "user_not_found", "User not found")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":     strconv.Itoa(u.ID),
		"name":   u.Name,
		"email":  u.Email,
		"phone":  u.Phone,
		"active": u.Active,
	})
}

func writeWazzupError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, wz.ErrAccountNotConnected):
		notFound(c, "wazzup_account_not_connected", "Аккаунт Wazzup не подключён — нажмите «Подключить» в настройках каналов мессенджера")
	case errors.Is(err, wz.ErrAccountNotConfigured):
		notFound(c, "wazzup_account_not_configured", "Аккаунт Wazzup не настроен на сервере")
	case errors.Is(err, wz.ErrBadRequest):
		badRequest(c, err.Error())
	case errors.Is(err, wz.ErrUnauthorized):
		unauthorized(c, "wazzup unauthorized")
	case errors.Is(err, wz.ErrNotFound), errors.Is(err, wz.ErrDisabled):
		notFound(c, "wazzup_integration_not_found", "Integration not found")
	case errors.Is(err, wz.ErrUsersSync):
		c.JSON(http.StatusBadGateway, gin.H{"error": "Wazzup users sync failed"})
	case errors.Is(err, wz.ErrUpstream):
		c.JSON(http.StatusBadGateway, gin.H{"error": "wazzup upstream error"})
	default:
		internalError(c, fallback)
	}
}

func tokenPrefix(token string) string {
	t := strings.TrimSpace(token)
	if len(t) > 6 {
		return t[:6] + "***"
	}
	if t == "" {
		return ""
	}
	return t + "***"
}

// GET /integrations/wazzup/accounts — аккаунты Wazzup и их состояние.
//
// can_add_channels — можно ли добавлять номера прямо из CRM (встроенное окно
// White Label). Это доступно только дочернему аккаунту; номера основного
// подключаются в кабинете Wazzup.
func (h *WazzupHandler) Accounts(c *gin.Context) {
	accounts, err := h.svc.Accounts(c.Request.Context())
	if err != nil {
		writeWazzupError(c, err, "failed to list wazzup accounts")
		return
	}
	items := make([]gin.H, 0, len(accounts))
	for _, a := range accounts {
		items = append(items, gin.H{
			"account":          a.Account,
			"title":            a.Title,
			"partner":          a.Partner,
			"connected":        a.Connected,
			"enabled":          a.Enabled,
			"webhook_url":      a.WebhookURL,
			"can_add_channels": a.Partner && h.wl != nil && h.wl.Configured(),
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
