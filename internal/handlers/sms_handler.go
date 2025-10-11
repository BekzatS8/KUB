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

// SendSMSHandler — отправка SMS
// @Summary      Отправить SMS
// @Description  Отправляет SMS с кодом подтверждения на указанный номер
// @Tags         SMS
// @Accept       json
// @Produce      json
// @Param        input  body  object{document_id=int64,phone=string}  true  "Данные для отправки SMS"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /sms/send [post]
func (h *SMSHandler) SendSMSHandler(c *gin.Context) {
	var input struct {
		DocumentID int64  `json:"document_id"`
		Phone      string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	if err := h.Service.SendSMS(input.DocumentID, input.Phone); err != nil {
		fmt.Printf("❌ Failed to send SMS: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "SMS sent"})
}

// ResendSMSHandler — повторная отправка SMS
// @Summary      Повторная отправка SMS
// @Description  Повторно отправляет SMS по ID документа
// @Tags         SMS
// @Produce      json
// @Param        document_id  query  int64  true  "ID документа"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /sms/resend [get]
func (h *SMSHandler) ResendSMSHandler(c *gin.Context) {
	documentIDStr := c.Query("document_id")
	documentID, err := strconv.ParseInt(documentIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid document_id"})
		return
	}

	if err := h.Service.ResendSMS(documentID, ""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resend SMS"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "SMS resent"})
}

// ConfirmSMSHandler — подтверждение кода
// @Summary      Подтвердить SMS-код
// @Description  Подтверждает введённый код по ID документа
// @Tags         SMS
// @Accept       json
// @Produce      json
// @Param        input  body  object{document_id=int64,code=string}  true  "ID документа и код подтверждения"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /sms/confirm [post]
func (h *SMSHandler) ConfirmSMSHandler(c *gin.Context) {
	var input struct {
		DocumentID int64  `json:"document_id"`
		Code       string `json:"code"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	ok, err := h.Service.ConfirmCode(input.DocumentID, input.Code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Confirmation failed"})
		return
	}
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired code"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Code confirmed"})
}

// GetLatestSMSHandler — получить последнее SMS
// @Summary      Получить последнее SMS
// @Description  Возвращает последнее SMS по документу
// @Tags         SMS
// @Produce      json
// @Param        document_id  path  int64  true  "ID документа"
// @Success      200  {object}  models.SMSConfirmation
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /sms/{document_id} [get]
func (h *SMSHandler) GetLatestSMSHandler(c *gin.Context) {
	documentIDStr := c.Param("document_id")
	documentID, err := strconv.ParseInt(documentIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid document_id"})
		return
	}

	sms, err := h.Service.GetLatestByDocumentID(documentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch SMS"})
		return
	}
	if sms == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No SMS found"})
		return
	}

	c.JSON(http.StatusOK, sms)
}

// DeleteSMSHandler — удалить подтверждения
// @Summary      Удалить SMS-подтверждения
// @Description  Удаляет все SMS-подтверждения по документу
// @Tags         SMS
// @Produce      json
// @Param        document_id  path  int64  true  "ID документа"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /sms/{document_id} [delete]
func (h *SMSHandler) DeleteSMSHandler(c *gin.Context) {
	documentIDStr := c.Param("document_id")
	documentID, err := strconv.ParseInt(documentIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid document_id"})
		return
	}

	if err := h.Service.DeleteConfirmation(documentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete confirmations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Confirmations deleted"})
}
