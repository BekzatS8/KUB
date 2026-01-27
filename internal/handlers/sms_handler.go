package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
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

	c.JSON(http.StatusOK, gin.H{
		"message":      "SMS sent",
		"legal_notice": "Client received a legally binding signature request with full disclosure",
	})
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

	c.JSON(http.StatusOK, gin.H{
		"message":      "SMS resent",
		"legal_notice": "Client received a legally binding signature request",
	})
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

	// Получаем IP и User-Agent для юридической значимости
	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()

	// Используем новую версию с метаданными
	ok, err := h.Service.ConfirmCodeWithMetadata(input.DocumentID, input.Code, ip, userAgent)
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

	// Логируем юридически значимую подпись
	logSignatureEvent(c, input.DocumentID, input.Code, ip, userAgent)

	c.JSON(http.StatusOK, gin.H{
		"message":             "Document successfully signed",
		"legal_notice":        "The document has been legally signed and is now binding",
		"signature_timestamp": time.Now().Format(time.RFC3339),
		"signature_method":    "SMS OTP with legal consent",
		"ip_address":          ip,
	})
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
		LegalNotice string    `json:"legal_notice,omitempty"`
	}

	response := smsResponse{
		DocumentID:  sms.DocumentID,
		Phone:       maskPhone(sms.Phone),
		SentAt:      sms.SentAt,
		Confirmed:   sms.Confirmed,
		ConfirmedAt: sms.ConfirmedAt,
	}

	if sms.Confirmed {
		response.LegalNotice = "Document has been legally signed via SMS OTP"
	}

	c.JSON(http.StatusOK, response)
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

// Вспомогательная функция для логирования юридически значимой подписи
func logSignatureEvent(c *gin.Context, documentID int64, code string, ip, userAgent string) {
	// Маскируем код для безопасности
	maskedCode := ""
	if len(code) > 2 {
		maskedCode = code[:2] + strings.Repeat("*", len(code)-2)
	} else {
		maskedCode = "******"
	}

	// Определяем устройство
	deviceInfo := "Unknown"
	if strings.Contains(userAgent, "Mobile") {
		deviceInfo = "Mobile Device"
	} else if strings.Contains(userAgent, "Windows") {
		deviceInfo = "Windows PC"
	} else if strings.Contains(userAgent, "Mac") {
		deviceInfo = "Mac"
	}

	fmt.Printf("[SIGNATURE][LEGAL] Document %d signed via SMS OTP\n", documentID)
	fmt.Printf("[SIGNATURE][LEGAL] IP: %s, Device: %s\n", ip, deviceInfo)
	fmt.Printf("[SIGNATURE][LEGAL] Code used: %s\n", maskedCode)
	fmt.Printf("[SIGNATURE][LEGAL] Timestamp: %s\n", time.Now().Format(time.RFC3339))
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
