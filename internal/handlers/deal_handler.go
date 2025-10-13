package handlers

import (
	"net/http"
	"strconv"
	"time"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/services"

	"github.com/gin-gonic/gin"
)

type DealHandler struct {
	Service *services.DealService
}

func NewDealHandler(service *services.DealService) *DealHandler {
	return &DealHandler{Service: service}
}

// @Summary      Создание сделки
// @Description  Создает новую сделку, связанную с лидом. Продажник создаёт только СВОЮ сделку.
// @Tags         Deals
// @Accept       json
// @Produce      json
// @Param        deal  body      models.Deals  true  "Данные сделки (lead_id обязателен)"
// @Success      201   {object}  models.Deals
// @Failure      400   {object}  map[string]string
// @Failure      403   {object}  map[string]string
// @Failure      500   {object}  map[string]string
// @Router       /deals [post]
func (h *DealHandler) Create(c *gin.Context) {
	var deal models.Deals
	if err := c.ShouldBindJSON(&deal); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, roleID := getUserAndRole(c)
	// Аудит — только чтение
	if authz.IsReadOnly(roleID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "read-only role"})
		return
	}
	// Владелец сделки — тот, кто создал
	deal.OwnerID = userID
	if deal.Status == "" {
		deal.Status = "new"
	}
	if deal.CreatedAt.IsZero() {
		deal.CreatedAt = time.Now()
	}

	id, err := h.Service.Create(&deal)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	deal.ID = int(id)
	c.JSON(http.StatusCreated, deal)
}

// @Summary      Обновление сделки
// @Description  Обновляет данные сделки по ее ID (владелец или повышенные роли).
// @Tags         Deals
// @Accept       json
// @Produce      json
// @Param        id    path      int           true  "ID сделки"
// @Param        deal  body      models.Deals  true  "Новые данные сделки"
// @Success      200   {object}  models.Deals
// @Failure      400   {object}  map[string]string
// @Failure      403   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      500   {object}  map[string]string
// @Router       /deals/{id} [put]
func (h *DealHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	userID, roleID := getUserAndRole(c)
	if authz.IsReadOnly(roleID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "read-only role"})
		return
	}

	current, err := h.Service.GetByID(id)
	if err != nil || current == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deal not found"})
		return
	}
	// sales — только свою; elevated — любую
	if current.OwnerID != userID && !authz.IsElevated(roleID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	var body models.Deals
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	body.ID = id

	// запрещаем менять владельца руками, если не elevated
	if !authz.IsElevated(roleID) {
		body.OwnerID = current.OwnerID
	}

	if err := h.Service.Update(&body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	updated, _ := h.Service.GetByID(id)
	c.JSON(http.StatusOK, updated)
}

// @Summary      Получить сделку по ID
// @Description  Возвращает данные одной сделки. Sales видит только свою; остальные (ops/mgmt/admin/audit) — любую.
// @Tags         Deals
// @Produce      json
// @Param        id   path      int  true  "ID сделки"
// @Success      200  {object}  models.Deals
// @Failure      403  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /deals/{id} [get]
func (h *DealHandler) GetByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	userID, roleID := getUserAndRole(c)
	deal, err := h.Service.GetByID(id)
	if err != nil || deal == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deal not found"})
		return
	}
	if deal.OwnerID != userID && !authz.IsElevated(roleID) && roleID != authz.RoleAudit {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	c.JSON(http.StatusOK, deal)
}

// @Summary      Удалить сделку
// @Description  Удаляет сделку по ID. Audit запрещено; Sales — только свою; elevated — любую.
// @Tags         Deals
// @Param        id   path  int  true  "ID сделки"
// @Success      204  "No Content"
// @Failure      403  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /deals/{id} [delete]
func (h *DealHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	userID, roleID := getUserAndRole(c)
	if authz.IsReadOnly(roleID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "read-only role"})
		return
	}

	deal, err := h.Service.GetByID(id)
	if err != nil || deal == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deal not found"})
		return
	}
	if deal.OwnerID != userID && !authz.IsElevated(roleID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	if err := h.Service.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// @Summary      Список сделок с пагинацией
// @Description  Sales — видит только свои; ops/mgmt/admin/audit — все.
// @Tags         Deals
// @Produce      json
// @Param        page   query     int  false  "Номер страницы (по умолчанию 1)"
// @Param        size   query     int  false  "Размер страницы (по умолчанию 100)"
// @Success      200  {array}  models.Deals
// @Failure      500  {object}  map[string]string
// @Router       /deals [get]
func (h *DealHandler) List(c *gin.Context) {
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

	userID, roleID := getUserAndRole(c)

	var (
		deals []*models.Deals
	)
	if authz.IsElevated(roleID) || roleID == authz.RoleAudit {
		deals, err = h.Service.ListPaginated(size, offset)
	} else {
		deals, err = h.Service.ListMy(userID, size, offset)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve deals",
			"debug": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, deals)
}
