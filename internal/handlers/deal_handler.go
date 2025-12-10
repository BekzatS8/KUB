package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type DealHandler struct {
	Service *services.DealService
}

func NewDealHandler(service *services.DealService) *DealHandler {
	return &DealHandler{Service: service}
}

func (h *DealHandler) Create(c *gin.Context) {
	var deal models.Deals
	if err := c.ShouldBindJSON(&deal); err != nil {
		badRequest(c, "Invalid payload")
		return
	}

	userID, roleID := getUserAndRole(c)
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}
	deal.OwnerID = userID
	if deal.Status == "" {
		deal.Status = "new"
	}
	if deal.CreatedAt.IsZero() {
		deal.CreatedAt = time.Now()
	}

	id, err := h.Service.Create(&deal)
	if err != nil {
		if err.Error() == "client_id is required" {
			badRequest(c, "Client ID is required")
			return
		}
		internalError(c, "Failed to create deal")
		return
	}
	deal.ID = int(id)
	c.JSON(http.StatusCreated, deal)
}

func (h *DealHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}

	userID, roleID := getUserAndRole(c)
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	current, err := h.Service.GetByID(id)
	if err != nil || current == nil {
		notFound(c, DealNotFoundCode, "Deal not found")
		return
	}
	if current.OwnerID != userID && !authz.IsElevated(roleID) {
		forbidden(c, "Forbidden")
		return
	}

	var body models.Deals
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	body.ID = id
	if !authz.IsElevated(roleID) {
		body.OwnerID = current.OwnerID
	}

	if err := h.Service.Update(&body); err != nil {
		internalError(c, "Failed to update deal")
		return
	}
	updated, _ := h.Service.GetByID(id)
	c.JSON(http.StatusOK, updated)
}

func (h *DealHandler) GetByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}

	userID, roleID := getUserAndRole(c)
	deal, err := h.Service.GetByID(id)
	if err != nil || deal == nil {
		notFound(c, DealNotFoundCode, "Deal not found")
		return
	}
	if deal.OwnerID != userID && !authz.IsElevated(roleID) && roleID != authz.RoleAudit {
		forbidden(c, "Forbidden")
		return
	}
	c.JSON(http.StatusOK, deal)
}

func (h *DealHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, roleID := getUserAndRole(c)
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	deal, err := h.Service.GetByID(id)
	if err != nil || deal == nil {
		notFound(c, DealNotFoundCode, "Deal not found")
		return
	}
	if deal.OwnerID != userID && !authz.IsElevated(roleID) {
		forbidden(c, "Forbidden")
		return
	}

	if err := h.Service.Delete(id); err != nil {
		internalError(c, "Failed to delete deal")
		return
	}
	c.Status(http.StatusNoContent)
}

// --- UpdateStatus ---
type updateDealStatusRequest struct {
	To      string `json:"to" binding:"required"`
	Comment string `json:"comment"`
}

func (h *DealHandler) UpdateStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}

	var req updateDealStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}

	userID, roleID := getUserAndRole(c)
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	current, err := h.Service.GetByID(id)
	if err != nil || current == nil {
		notFound(c, DealNotFoundCode, "Deal not found")
		return
	}
	if current.OwnerID != userID && !authz.IsElevated(roleID) {
		forbidden(c, "Forbidden")
		return
	}

	if err := h.Service.UpdateStatus(id, req.To); err != nil {
		badRequest(c, "Invalid status")
		return
	}

	updated, _ := h.Service.GetByID(id)
	c.JSON(http.StatusOK, updated)
}

func (h *DealHandler) List(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	sizeStr := c.DefaultQuery("size", "100")

	page, _ := strconv.Atoi(pageStr)
	size, _ := strconv.Atoi(sizeStr)
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 100
	}
	offset := (page - 1) * size

	userID, roleID := getUserAndRole(c)
	var deals []*models.Deals
	var err error

	if authz.IsElevated(roleID) || roleID == authz.RoleAudit {
		deals, err = h.Service.ListPaginated(size, offset)
	} else {
		deals, err = h.Service.ListMy(userID, size, offset)
	}
	if err != nil {
		internalError(c, "Failed to retrieve deals")
		return
	}
	c.JSON(http.StatusOK, deals)
}
