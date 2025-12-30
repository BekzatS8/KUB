package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
	"turcompany/internal/models"
	"turcompany/internal/services"

	"github.com/gin-gonic/gin"
)

type SMSHandler struct {
	Service *services.SMS_Service
}

func NewSMSHandler(service *services.SMS_Service) *SMSHandler {
	_ = models.SMSConfirmation{}
	return &SMSHandler{Service: service}
}

func (h *SMSHandler) SendSMSHandler(c *gin.Context) {
	var input struct {
		DocumentID int64  `json:"document_id"`
		Phone      string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Invalid input")
		return
	}

	userID, roleID := getUserAndRole(c)
	if err := h.Service.SendSMS(input.DocumentID, input.Phone, userID, roleID); err != nil {
		if err == services.ErrResendThrottled || err == services.ErrTooManyAttempts {
			badRequest(c, err.Error())
			return
		}
		fmt.Printf("❌ Failed to send SMS: %v\n", err)
		internalError(c, "Failed to send SMS")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "SMS sent"})
}

func (h *SMSHandler) ResendSMSHandler(c *gin.Context) {
	var input struct {
		DocumentID int64  `json:"document_id" binding:"required"`
		Phone      string `json:"phone"` // опционально, если надо указать другой номер
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Invalid resend payload")
		return
	}

	userID, roleID := getUserAndRole(c)
	if err := h.Service.ResendSMS(input.DocumentID, input.Phone, userID, roleID); err != nil {
		if err == services.ErrResendThrottled {
			badRequest(c, err.Error())
			return
		}
		internalError(c, "Failed to resend SMS")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "SMS resent"})
}

func (h *SMSHandler) ConfirmSMSHandler(c *gin.Context) {
	var input struct {
		DocumentID int64  `json:"document_id"`
		Code       string `json:"code"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Invalid input")
		return
	}

	userID, roleID := getUserAndRole(c)
	ok, err := h.Service.ConfirmCode(input.DocumentID, input.Code, userID, roleID)
	if err != nil {
		if err == services.ErrCodeExpired || err == services.ErrCodeInvalid || err == services.ErrTooManyAttempts {
			badRequest(c, err.Error())
			return
		}
		internalError(c, "Confirmation failed")
		return
	}
	if !ok {
		badRequest(c, "Invalid or expired code")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Code confirmed"})
}

func (h *SMSHandler) GetLatestSMSHandler(c *gin.Context) {
	documentIDStr := c.Param("document_id")
	documentID, err := strconv.ParseInt(documentIDStr, 10, 64)
	if err != nil {
		badRequest(c, "Invalid document id")
		return
	}

	sms, err := h.Service.GetLatestByDocumentID(documentID)
	if err != nil {
		internalError(c, "Failed to fetch SMS")
		return
	}
	if sms == nil {
		notFound(c, ValidationFailed, "No SMS found")
		return
	}

	type smsResponse struct {
		DocumentID  int64     `json:"document_id"`
		Phone       string    `json:"phone"`
		SentAt      time.Time `json:"sent_at"`
		Confirmed   bool      `json:"confirmed"`
		ConfirmedAt time.Time `json:"confirmed_at"`
	}
	c.JSON(http.StatusOK, smsResponse{
		DocumentID:  sms.DocumentID,
		Phone:       maskPhone(sms.Phone),
		SentAt:      sms.SentAt,
		Confirmed:   sms.Confirmed,
		ConfirmedAt: sms.ConfirmedAt,
	})
}

func (h *SMSHandler) DeleteSMSHandler(c *gin.Context) {
	documentIDStr := c.Param("document_id")
	documentID, err := strconv.ParseInt(documentIDStr, 10, 64)
	if err != nil {
		badRequest(c, "Invalid document id")
		return
	}

	if err := h.Service.DeleteConfirmation(documentID); err != nil {
		internalError(c, "Failed to delete confirmations")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Confirmations deleted"})
}

func maskPhone(phone string) string {
	trimmed := ""
	for _, r := range phone {
		if r != ' ' {
			trimmed += string(r)
		}
	}
	if len(trimmed) <= 4 {
		return "****"
	}
	return fmt.Sprintf("****%s", trimmed[len(trimmed)-4:])
}
