package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
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
	if authz.CanManageSystem(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}
	if roleID == authz.RoleSales {
		deal.OwnerID = userID
	}
	if deal.Status == "" {
		deal.Status = "new"
	}
	if deal.ClientID <= 0 {
		badRequest(c, "Client ID is required")
		return
	}
	if deal.ClientType == "" {
		badRequest(c, "Client type is required")
		return
	}
	if deal.CreatedAt.IsZero() {
		deal.CreatedAt = time.Now()
	}

	id, err := h.Service.Create(&deal, userID, roleID)
	if err != nil {
		if err.Error() == "lead_id is required" {
			badRequest(c, "Lead ID is required")
			return
		}
		if errors.Is(err, services.ErrClientIDRequired) {
			badRequest(c, "Client ID is required")
			return
		}
		if errors.Is(err, services.ErrClientTypeRequired) {
			badRequest(c, "Client type is required")
			return
		}
		if errors.Is(err, services.ErrClientTypeMismatch) {
			badRequest(c, "client_id and client_type mismatch")
			return
		}
		if strings.Contains(err.Error(), "invalid client_type") {
			badRequest(c, err.Error())
			return
		}
		if err.Error() == "amount must be greater than 0" {
			badRequest(c, "Amount must be greater than 0")
			return
		}
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
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
	if authz.CanManageSystem(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	current, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || current == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, DealNotFoundCode, "Deal not found")
		return
	}

	var body models.Deals
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	if body.ClientID <= 0 {
		badRequest(c, "Client ID is required")
		return
	}
	if body.ClientType == "" {
		badRequest(c, "Client type is required")
		return
	}
	body.ID = id
	if err := h.Service.Update(&body, userID, roleID); err != nil {
		if errors.Is(err, services.ErrClientIDRequired) {
			badRequest(c, "Client ID is required")
			return
		}
		if errors.Is(err, services.ErrClientTypeRequired) {
			badRequest(c, "Client type is required")
			return
		}
		if errors.Is(err, services.ErrClientTypeMismatch) {
			badRequest(c, "client_id and client_type mismatch")
			return
		}
		if strings.Contains(err.Error(), "invalid client_type") {
			badRequest(c, err.Error())
			return
		}
		if err.Error() == "amount must be greater than 0" {
			badRequest(c, "Amount must be greater than 0")
			return
		}
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
			return
		}
		internalError(c, "Failed to update deal")
		return
	}
	updated, _ := h.Service.GetByID(id, userID, roleID)
	c.JSON(http.StatusOK, updated)
}

func (h *DealHandler) GetByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}

	userID, roleID := getUserAndRole(c)
	if authz.CanManageSystem(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	deal, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || deal == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, DealNotFoundCode, "Deal not found")
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
	if authz.CanManageSystem(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	deal, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || deal == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, DealNotFoundCode, "Deal not found")
		return
	}

	if err := h.Service.Delete(id, userID, roleID); err != nil {
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
			return
		}
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
	if authz.CanManageSystem(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	current, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || current == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, DealNotFoundCode, "Deal not found")
		return
	}

	if err := h.Service.UpdateStatus(id, req.To, userID, roleID); err != nil {
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
			return
		}
		badRequest(c, "Invalid status")
		return
	}

	updated, _ := h.Service.GetByID(id, userID, roleID)
	c.JSON(http.StatusOK, updated)
}

func (h *DealHandler) List(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if authz.CanManageSystem(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	if roleID == authz.RoleSales {
		forbidden(c, "sales cannot access full list")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "100"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 100
	}
	offset := (page - 1) * size

	deals, err := h.Service.ListForRole(userID, roleID, size, offset)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Failed to retrieve deals")
		return
	}
	c.JSON(http.StatusOK, deals)
}

// GET /deals/my?page=&size=
func (h *DealHandler) ListMy(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if authz.CanManageSystem(roleID) {
		forbidden(c, "Forbidden")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "100"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 100
	}
	offset := (page - 1) * size

	deals, err := h.Service.ListMy(userID, size, offset)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Failed to retrieve deals")
		return
	}
	c.JSON(http.StatusOK, deals)
}
