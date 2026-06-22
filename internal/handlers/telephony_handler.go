package handlers

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

// TelephonyHandler serves both the public Binotel webhook and the private telephony API.
type TelephonyHandler struct {
	svc *services.TelephonyService
}

// NewTelephonyHandler creates a TelephonyHandler.
func NewTelephonyHandler(svc *services.TelephonyService) *TelephonyHandler {
	return &TelephonyHandler{svc: svc}
}

// ── Public webhook ────────────────────────────────────────────────────────────

// BinotelWebhook handles POST /api/v1/integrations/binotel/webhook
// No JWT. Optionally protected by BINOTEL_WEBHOOK_SECRET.
func (h *TelephonyHandler) BinotelWebhook(c *gin.Context) {
	// Validate secret if configured.
	secret := h.svc.WebhookSecret()
	if secret != "" {
		// Accept via header or query param.
		provided := c.GetHeader("X-Binotel-Secret")
		if provided == "" {
			provided = c.Query("token")
		}
		if provided != secret {
			log.Printf("integration=binotel operation=webhook status=unauthorized ip=%s", c.ClientIP())
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
	} else {
		log.Printf("integration=binotel operation=webhook status=warn no_secret_configured ip=%s", c.ClientIP())
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20)) // 1 MB limit
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}
	if len(body) == 0 {
		body = []byte(`{}`)
	}

	callID, isNew, err := h.svc.HandleBinotelWebhook(c.Request.Context(), body)
	if err != nil {
		log.Printf("integration=binotel operation=webhook status=error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "ok",
		"call_id":  callID,
		"is_new":   isNew,
	})
}

// ── Private API ───────────────────────────────────────────────────────────────

// ListCalls handles GET /api/v1/telephony/calls
func (h *TelephonyHandler) ListCalls(c *gin.Context) {
	roleIDVal, _ := c.Get("role_id")
	roleIDInt, _ := roleIDVal.(int)
	userIDVal, _ := c.Get("user_id")
	userIDInt, _ := userIDVal.(int)

	filter := models.TelephonyCallListFilter{
		Status: strings.TrimSpace(c.Query("status")),
		Phone:  strings.TrimSpace(c.Query("phone")),
		Limit:  parseQueryInt(c.Query("limit"), 50),
		Offset: parseQueryInt(c.Query("offset"), 0),
	}
	if v := strings.TrimSpace(c.Query("date_from")); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.DateFrom = &t
		}
	}
	if v := strings.TrimSpace(c.Query("date_to")); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.DateTo = &t
		}
	}
	if v := c.Query("manager_id"); v != "" {
		if id, err := strconv.Atoi(v); err == nil && id > 0 {
			filter.ManagerID = &id
		}
	}

	calls, total, err := h.svc.ListCalls(c.Request.Context(), userIDInt, roleIDInt, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if calls == nil {
		calls = []*models.TelephonyCallResponse{}
	}
	c.JSON(http.StatusOK, gin.H{"items": calls, "total": total})
}

// GetCall handles GET /api/v1/telephony/calls/:id
func (h *TelephonyHandler) GetCall(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	roleIDVal, _ := c.Get("role_id")
	roleIDInt, _ := roleIDVal.(int)
	userIDVal, _ := c.Get("user_id")
	userIDInt, _ := userIDVal.(int)

	call, err := h.svc.GetCall(c.Request.Context(), userIDInt, roleIDInt, id)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if call == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, call)
}

// ListClientCalls handles GET /api/v1/clients/:id/calls
func (h *TelephonyHandler) ListClientCalls(c *gin.Context) {
	clientID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || clientID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid client id"})
		return
	}
	roleIDVal, _ := c.Get("role_id")
	roleIDInt, _ := roleIDVal.(int)
	userIDVal, _ := c.Get("user_id")
	userIDInt, _ := userIDVal.(int)

	limit := parseQueryInt(c.Query("limit"), 20)
	offset := parseQueryInt(c.Query("offset"), 0)

	calls, total, err := h.svc.ListClientCalls(c.Request.Context(), userIDInt, roleIDInt, clientID, limit, offset)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if calls == nil {
		calls = []*models.TelephonyCallResponse{}
	}
	c.JSON(http.StatusOK, gin.H{"items": calls, "total": total})
}

// ListLeadCalls handles GET /api/v1/leads/:id/calls
func (h *TelephonyHandler) ListLeadCalls(c *gin.Context) {
	leadID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || leadID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid lead id"})
		return
	}
	roleIDVal, _ := c.Get("role_id")
	roleIDInt, _ := roleIDVal.(int)
	userIDVal, _ := c.Get("user_id")
	userIDInt, _ := userIDVal.(int)

	limit := parseQueryInt(c.Query("limit"), 20)
	offset := parseQueryInt(c.Query("offset"), 0)

	calls, total, err := h.svc.ListLeadCalls(c.Request.Context(), userIDInt, roleIDInt, leadID, limit, offset)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if calls == nil {
		calls = []*models.TelephonyCallResponse{}
	}
	c.JSON(http.StatusOK, gin.H{"items": calls, "total": total})
}

// InitiateCall handles POST /api/v1/telephony/calls/initiate
// Body: { "phone": "+77001234567", "manager_id": 5 }
// manager_id is optional — if omitted the caller's own extension is used.
// Requires telephony.view. Returns { "general_call_id": "..." }.
func (h *TelephonyHandler) InitiateCall(c *gin.Context) {
	roleIDVal, _ := c.Get("role_id")
	roleIDInt, _ := roleIDVal.(int)
	userIDVal, _ := c.Get("user_id")
	userIDInt, _ := userIDVal.(int)

	var body struct {
		Phone     string `json:"phone"`
		ManagerID *int   `json:"manager_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	phone := strings.TrimSpace(body.Phone)
	if phone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone is required"})
		return
	}

	generalCallID, err := h.svc.InitiateCall(c.Request.Context(), userIDInt, roleIDInt, phone, body.ManagerID)
	if err != nil {
		log.Printf("telephony: initiate_call error: %v", err)
		if errors.Is(err, services.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"general_call_id": generalCallID})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parseQueryInt(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return def
	}
	return v
}

