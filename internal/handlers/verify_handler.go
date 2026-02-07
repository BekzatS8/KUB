package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
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
	var req models.RegisterConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid confirmation payload")
		return
	}

	ok, err := h.verification.Confirm(req.UserID, req.Code)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrCodeExpired):
			badRequest(c, "Code expired, please resend")
			return
		case errors.Is(err, services.ErrTooManyAttempts):
			writeError(c, http.StatusTooManyRequests, ValidationFailed, "Too many attempts, please resend")
			return
		case errors.Is(err, services.ErrCodeInvalid):
			badRequest(c, "Invalid code")
			return
		case errors.Is(err, services.ErrNoPendingVerification):
			badRequest(c, "No pending verification")
			return
		case errors.Is(err, services.ErrAlreadyVerified):
			badRequest(c, "Already verified")
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
	var req models.RegisterResendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid resend payload")
		return
	}

	key := fmt.Sprintf("%d:%s", req.UserID, c.ClientIP())
	if !h.allowResend(key) {
		writeError(c, http.StatusTooManyRequests, ValidationFailed, "Too many requests, try later")
		return
	}

	if err := h.verification.Resend(req.UserID); err != nil {
		if errors.Is(err, services.ErrResendThrottled) {
			writeError(c, http.StatusTooManyRequests, ValidationFailed, "Too many requests, try later")
			return
		}
		switch {
		case errors.Is(err, services.ErrAlreadyVerified):
			badRequest(c, "Already verified")
		case errors.Is(err, services.ErrNoPendingVerification):
			badRequest(c, "No pending verification")
		default:
			internalError(c, "Failed to resend verification")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Verification code sent"})
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

// DebugLatest is a DEV-only endpoint to inspect the latest verification record.
func (h *VerifyHandler) DebugLatest(c *gin.Context) {
	if gin.Mode() == gin.ReleaseMode {
		c.Status(http.StatusNotFound)
		return
	}
	userID, err := strconv.Atoi(c.Query("user_id"))
	if err != nil || userID <= 0 {
		badRequest(c, "Invalid user_id")
		return
	}
	v, user, err := h.verification.Latest(userID)
	if err != nil {
		if errors.Is(err, services.ErrNoPendingVerification) {
			badRequest(c, "No pending verification")
			return
		}
		internalError(c, "Failed to fetch verification")
		return
	}
	if v == nil {
		badRequest(c, "No pending verification")
		return
	}
	status := "pending"
	if v.Confirmed {
		status = "confirmed"
	} else if time.Now().After(v.ExpiresAt) {
		status = "expired"
	}
	codeHashPrefix := v.CodeHash
	if len(codeHashPrefix) > 8 {
		codeHashPrefix = codeHashPrefix[:8]
	}
	c.JSON(http.StatusOK, gin.H{
		"user_id":          v.UserID,
		"email":            user.Email,
		"channel":          "email",
		"status":           status,
		"expires_at":       v.ExpiresAt,
		"attempts":         v.Attempts,
		"created_at":       v.SentAt,
		"last_resend_at":   v.LastResendAt,
		"resend_count":     v.ResendCount,
		"code_hash_prefix": codeHashPrefix,
		"code_hash_len":    len(v.CodeHash),
	})
}
