package handlers

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/authz"
	wz "turcompany/internal/integrations/wazzup"
)

type WazzupService interface {
	Setup(ctx context.Context, ownerUserID int, webhooksBaseURL string, apiKey string, enabled bool) (*wz.SetupResponse, error)
	GetIframeURL(ctx context.Context, ownerUserID int, phone string, leadID int, clientID int) (string, error)
	HandleWebhook(ctx context.Context, token string, authHeader string, payload []byte) (leadID int, created bool, err error)
}

type WazzupHandler struct {
	svc WazzupService
}

func NewWazzupHandler(svc WazzupService) *WazzupHandler {
	return &WazzupHandler{svc: svc}
}

type wazzupSetupRequest struct {
	WebhooksBaseURL string `json:"webhooks_base_url"`
	APIKey          string `json:"api_key"`
	Enabled         bool   `json:"enabled"`
}

type wazzupIframeRequest struct {
	Phone    string `json:"phone"`
	LeadID   int    `json:"lead_id"`
	ClientID int    `json:"client_id"`
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
		log.Printf("[WAZZUP][webhook] token=%s err=%v payload_prefix=%q", tokenPrefix(token), err, payloadPreview)
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
	if roleID != authz.RoleManagement && roleID != authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	var req wazzupSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	if strings.TrimSpace(req.WebhooksBaseURL) == "" {
		badRequest(c, "webhooks_base_url is required")
		return
	}
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("WAZZUP_API_KEY_DEFAULT"))
	}
	if apiKey == "" {
		badRequest(c, "api_key is required")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	resp, err := h.svc.Setup(ctx, userID, req.WebhooksBaseURL, apiKey, req.Enabled)
	if err != nil {
		log.Printf("[WAZZUP][setup] user_id=%d base_url=%q enabled=%v err=%v", userID, req.WebhooksBaseURL, req.Enabled, err)
		if errors.Is(err, wz.ErrUpstream) {
			log.Printf("[WAZZUP][setup] upstream_error=%v", err)
		}
		switch {
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

func (h *WazzupHandler) Iframe(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	var req wazzupIframeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()

	url, err := h.svc.GetIframeURL(ctx, userID, req.Phone, req.LeadID, req.ClientID)
	if err != nil {
		log.Printf("[WAZZUP][iframe] user_id=%d lead_id=%d client_id=%d phone=%q err=%v", userID, req.LeadID, req.ClientID, req.Phone, err)
		switch {
		case errors.Is(err, wz.ErrBadRequest):
			badRequest(c, err.Error())
		case errors.Is(err, wz.ErrNotFound), errors.Is(err, wz.ErrDisabled):
			notFound(c, "wazzup_integration_not_found", "Integration not found")
		case errors.Is(err, wz.ErrUpstream):
			c.JSON(http.StatusBadGateway, gin.H{"error": "wazzup upstream error"})
		default:
			internalError(c, "failed to get iframe")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"iframe_url": url})
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
