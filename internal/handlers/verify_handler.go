package handlers

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/services"
)

type VerifyHandler struct {
	SMS *services.SMS_Service

	mu          sync.Mutex
	lastResends map[string]time.Time
}

const resendMinInterval = time.Minute

func NewVerifyHandler(s *services.SMS_Service) *VerifyHandler {
	return &VerifyHandler{
		SMS:         s,
		lastResends: make(map[string]time.Time),
	}
}

func (h *VerifyHandler) ConfirmUser(c *gin.Context) {
	var req struct {
		UserID int    `json:"user_id" binding:"required"`
		Code   string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid code")
		return
	}

	ok, err := h.SMS.ConfirmUserCode(req.UserID, req.Code)
	if err != nil {
		switch err {
		case services.ErrCodeExpired:
			badRequest(c, "Code expired, please resend")
			return
		case services.ErrTooManyAttempts:
			badRequest(c, "Too many attempts, please resend")
			return
		case services.ErrCodeInvalid:
			badRequest(c, "Invalid code")
			return
		default:
			internalError(c, "Confirmation failed")
			return
		}
	}
	if !ok {
		badRequest(c, "Invalid or expired code")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Phone verified"})
}

func (h *VerifyHandler) ResendUser(c *gin.Context) {
	var req struct {
		UserID int `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid code")
		return
	}

	key := fmt.Sprintf("%d:%s", req.UserID, c.ClientIP())
	if !h.allowResend(key) {
		writeError(c, http.StatusTooManyRequests, ValidationFailed, "Too many requests, try later")
		return
	}

	if err := h.SMS.ResendUserSMS(req.UserID); err != nil {
		if err == services.ErrResendThrottled {
			writeError(c, http.StatusTooManyRequests, ValidationFailed, "Too many requests, try later")
			return
		}
		badRequest(c, "Invalid code")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "SMS sent"})
}

func (h *VerifyHandler) allowResend(key string) bool {
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	if last, ok := h.lastResends[key]; ok && now.Sub(last) < resendMinInterval {
		return false
	}
	h.lastResends[key] = now
	return true
}
