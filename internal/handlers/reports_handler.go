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

func (h *ReportHandler) GetSummary(c *gin.Context) {
	data, err := h.Service.GetSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

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
