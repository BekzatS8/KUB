package handlers

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"time"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type LeadHandler struct {
	Service *services.LeadService
}

func NewLeadHandler(service *services.LeadService) *LeadHandler {
	return &LeadHandler{Service: service}
}

// @Summary      Создать лид
// @Description  Создает нового клиента (лида)
// @Tags         Leads
// @Accept       json
// @Produce      json
// @Param        lead  body      models.Leads  true  "Данные нового лида"
// @Success      201   {object}  models.Leads
// @Failure      400   {object}  map[string]string
// @Failure      500   {object}  map[string]string
// @Router       /leads [post]
func (h *LeadHandler) Create(c *gin.Context) {
	var lead models.Leads

	// Привязка JSON к модели
	if err := c.ShouldBindJSON(&lead); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Проверка поля owner_id
	if lead.OwnerID == 0 {
		c.JSON(400, gin.H{"error": "owner_id is required and must be an integer"})
		return
	}

	// Установка временной метки
	lead.CreatedAt = time.Now()

	// Сохранение лида в базе
	if err := h.Service.Create(&lead); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// Успешное создание
	c.Status(201)
}

// @Summary      Обновить лид
// @Description  Обновляет данные лида по ID
// @Tags         Leads
// @Accept       json
// @Produce      json
// @Param        id    path      int           true  "ID Лида"
// @Param        lead  body      models.Leads  true  "Обновленные данные"
// @Success      200   {object}  models.Leads
// @Failure      400   {object}  map[string]string
// @Router       /leads/{id} [put]
func (h *LeadHandler) Update(c *gin.Context) {
	idStr := c.Param("id")
	var lead models.Leads
	if err := c.ShouldBindJSON(&lead); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	id, _ := strconv.Atoi(idStr)
	lead.ID = id
	if err := h.Service.Update(&lead); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.Status(200)
}

// @Summary      Получить лид по ID
// @Description  Возвращает данные одного лида
// @Tags         Leads
// @Produce      json
// @Param        id   path      int  true  "ID Лида"
// @Success      200  {object}  models.Leads
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /leads/{id} [get]
func (h *LeadHandler) GetByID(c *gin.Context) {
	idStr := c.Param("id")
	fmt.Println(idStr)
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid ID format"})
		return
	}

	lead, err := h.Service.GetByID(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "Lead not found"})
		return
	}
	// Если всё прошло успешно, возвращаем JSON ответ
	c.JSON(200, lead)
}

// @Summary      Удалить лида
// @Description  Удаляет клиента по ID
// @Tags         Leads
// @Param        id   path  int  true  "ID Лида"
// @Success      204  "No Content"
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /leads/{id} [delete]
func (h *LeadHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid ID format"})
		return
	}

	if err := h.Service.Delete(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.Status(204) // No Content
}

// ConvertLeadRequest используется только для Swagger
type ConvertLeadRequest struct {
	Amount   string `json:"amount" example:"50000"`
	Currency string `json:"currency" example:"USD"`
}

// ConvertToDeal godoc
// @Summary Конвертировать лид в сделку
// @Description Создает сделку на основе существующего лида
// @Tags Leads
// @Accept json
// @Produce json
// @Param id path int true "ID лида"
// @Param request body ConvertLeadRequest true "Данные для сделки"
// @Success 201 {object} models.Deals
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /leads/{id}/convert [put]
func (h *LeadHandler) ConvertToDeal(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid ID format"})
		return
	}

	var req struct {
		Amount   string `json:"amount" binding:"required"`
		Currency string `json:"currency" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Вызов ConvertLeadToDeal из LeadService
	deal, err := h.Service.ConvertLeadToDeal(id, req.Amount, req.Currency)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(201, deal)
}

// @Summary      Get all leads with pagination
// @Description  Returns a list of all leads with pagination
// @Tags         Leads
// @Produce      json
// @Param        page   query     int  false  "Page number (default 1)"
// @Param        size   query     int  false  "Page size (default 100)"
// @Success      200  {array}  models.Leads
// @Failure      500  {object}  map[string]string
// @Router       /leads [get]
func (h *LeadHandler) List(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	sizeStr := c.DefaultQuery("size", "100")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	size, err := strconv.Atoi(sizeStr)
	if err != nil || size < 1 {
		size = 100
	}

	offset := (page - 1) * size

	leads, err := h.Service.ListPaginated(size, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list leads"})
		return
	}
	c.JSON(http.StatusOK, leads)
}
