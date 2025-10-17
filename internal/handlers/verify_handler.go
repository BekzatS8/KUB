package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"turcompany/internal/services"
)

type VerifyHandler struct {
	SMS *services.SMS_Service
}

func NewVerifyHandler(s *services.SMS_Service) *VerifyHandler { return &VerifyHandler{SMS: s} }

func (h *VerifyHandler) ConfirmUser(c *gin.Context) {
	var req struct {
		UserID int    `json:"user_id" binding:"required"`
		Code   string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ok, err := h.SMS.ConfirmUserCode(req.UserID, req.Code)
	if err != nil {
		switch err {
		case services.ErrCodeExpired:
			c.JSON(http.StatusBadRequest, gin.H{"error": "code expired, please resend"})
			return
		case services.ErrTooManyAttempts:
			c.JSON(http.StatusBadRequest, gin.H{"error": "too many attempts, please resend"})
			return
		case services.ErrCodeInvalid:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid code"})
			return
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "confirmation failed"})
			return
		}
	}
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired code"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Phone verified"})
}

func (h *VerifyHandler) ResendUser(c *gin.Context) {
	var req struct {
		UserID int    `json:"user_id" binding:"required"`
		Phone  string `json:"phone" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.SMS.ResendUserSMS(req.UserID, req.Phone); err != nil {
		if err == services.ErrResendThrottled {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many requests, try later"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "SMS sent"})
}
