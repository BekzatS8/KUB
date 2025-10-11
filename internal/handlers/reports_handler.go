package handlers

import (
	"net/http"
	"strconv"
	"turcompany/internal/services"

	"github.com/gin-gonic/gin"
)

type ReportHandler struct {
	Service *services.ReportService
}

func NewReportHandler(service *services.ReportService) *ReportHandler {
	return &ReportHandler{Service: service}
}

// @Summary Сводный отчет
// @Description Выводит общее количество лидов и сделок.
// @Tags Reports
// @Produce json
// @Success 200 {object} map[string]int
// @Failure 500 {object} map[string]string
// @Router /reports/summary [get]
func (h *ReportHandler) GetSummary(c *gin.Context) {
	data, err := h.Service.GetSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

// @Summary Фильтрация лидов
// @Description Фильтрует лидов по статусу, владельцу и сортирует.
// @Tags Reports
// @Produce json
// @Param status query string false "Статус лида"
// @Param owner_id query int false "ID владельца"
// @Param sort_by query string false "Поле сортировки (created_at, owner_id, status)"
// @Param order query string false "Порядок сортировки (asc, desc)"
// @Param page query int false "Номер страницы"
// @Param size query int false "Размер страницы"
// @Success 200 {array} models.Leads
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /reports/leads/filter [get]
func (h *ReportHandler) FilterLeads(c *gin.Context) {
	status := c.Query("status")
	ownerID, _ := strconv.Atoi(c.DefaultQuery("owner_id", "0"))
	sortBy := c.DefaultQuery("sort_by", "created_at")
	order := c.DefaultQuery("order", "desc")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "100"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 100
	}
	offset := (page - 1) * size

	leads, err := h.Service.FilterLeads(status, ownerID, sortBy, order, size, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, leads)
}

// @Summary Фильтрация сделок
// @Description Фильтрует сделки по статусу, дате, валюте, сумме и сортирует.
// @Tags Reports
// @Produce json
// @Param status query string false "Статус сделки"
// @Param from query string false "Дата с (yyyy-mm-dd)"
// @Param to query string false "Дата по (yyyy-mm-dd)"
// @Param currency query string false "Валюта (например, USD, KZT)"
// @Param amount_min query number false "Минимальная сумма"
// @Param amount_max query number false "Максимальная сумма"
// @Param sort_by query string false "Поле сортировки (created_at, amount, currency, status)"
// @Param order query string false "Порядок сортировки (asc, desc)"
// @Param page query int false "Номер страницы"
// @Param size query int false "Размер страницы"
// @Success 200 {array} models.Deals
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /reports/deals/filter [get]
func (h *ReportHandler) FilterDeals(c *gin.Context) {
	status := c.Query("status")
	from := c.Query("from")
	to := c.Query("to")
	currency := c.Query("currency")

	sortBy := c.DefaultQuery("sort_by", "created_at")
	order := c.DefaultQuery("order", "desc")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "100"))
	amountMin, _ := strconv.ParseFloat(c.DefaultQuery("amount_min", "0"), 64)
	amountMax, _ := strconv.ParseFloat(c.DefaultQuery("amount_max", "0"), 64)

	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 100
	}
	offset := (page - 1) * size

	deals, err := h.Service.FilterDeals(status, from, to, currency, amountMin, amountMax, sortBy, order, size, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, deals)
}
