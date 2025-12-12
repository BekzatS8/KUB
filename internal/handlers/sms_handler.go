package handlers

import (
	"fmt"
	"net/http"
	"strconv"
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

	if err := h.Service.SendSMS(input.DocumentID, input.Phone); err != nil {
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

	if err := h.Service.ResendSMS(input.DocumentID, input.Phone); err != nil {
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

	ok, err := h.Service.ConfirmCode(input.DocumentID, input.Code)
	if err != nil {
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

	c.JSON(http.StatusOK, sms)
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
