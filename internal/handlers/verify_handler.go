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
	verification *services.UserVerificationService

	mu          sync.Mutex
	lastResends map[string]time.Time
}

const resendMinInterval = time.Minute

func NewVerifyHandler(s *services.UserVerificationService) *VerifyHandler {
	return &VerifyHandler{
		verification: s,
		lastResends:  make(map[string]time.Time),
	}
}

func (h *VerifyHandler) ConfirmUser(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
		Code  string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid code")
		return
	}

	ok, err := h.verification.Confirm(req.Email, req.Code)
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
	c.JSON(http.StatusOK, gin.H{"message": "Email verified"})
}

func (h *VerifyHandler) ResendUser(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid code")
		return
	}

	key := fmt.Sprintf("%s:%s", req.Email, c.ClientIP())
	if !h.allowResend(key) {
		writeError(c, http.StatusTooManyRequests, ValidationFailed, "Too many requests, try later")
		return
	}

	if err := h.verification.Resend(req.Email); err != nil {
		if err == services.ErrResendThrottled {
			writeError(c, http.StatusTooManyRequests, ValidationFailed, "Too many requests, try later")
			return
		}
		badRequest(c, "Invalid code")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Email sent"})
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
