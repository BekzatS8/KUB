package handlers

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type LeadHandler struct {
	Service *services.LeadService
}

func NewLeadHandler(service *services.LeadService) *LeadHandler {
	return &LeadHandler{Service: service}
}

func (h *LeadHandler) Create(c *gin.Context) {
	var lead models.Leads
	if err := c.ShouldBindJSON(&lead); err != nil {
		badRequest(c, "Invalid payload")
		return
	}

	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	// статус по умолчанию, owner и финальная логика — внутри сервиса
	if lead.Status == "" {
		lead.Status = "new"
	}

	id, err := h.Service.Create(&lead, userID, roleID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
			return
		}
		internalError(c, "Failed to create lead")
		return
	}
	lead.ID = int(id)
	c.JSON(http.StatusCreated, lead)
}

func (h *LeadHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}

	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
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
		notFound(c, LeadNotFoundCode, "Lead not found")
		return
	}

	var body models.Leads
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	body.ID = id
	if err := h.Service.Update(&body, userID, roleID); err != nil {
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
			return
		}
		internalError(c, "Failed to update lead")
		return
	}
	updated, _ := h.Service.GetByID(id, userID, roleID)
	c.JSON(200, updated)
}

func (h *LeadHandler) GetByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}

	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	lead, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || lead == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, LeadNotFoundCode, "Lead not found")
		return
	}
	c.JSON(200, lead)
}

func (h *LeadHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}

	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	lead, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || lead == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, LeadNotFoundCode, "Lead not found")
		return
	}

	if err := h.Service.Delete(id, userID, roleID); err != nil {
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
			return
		}
		internalError(c, "Failed to delete lead")
		return
	}
	c.Status(204)
}

// --- Assign ---
type assignLeadRequest struct {
	AssigneeID int    `json:"assignee_id" binding:"required"`
	Comment    string `json:"comment"`
}

func (h *LeadHandler) Assign(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}

	var req assignLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}

	actorID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	lead, err := h.Service.GetByID(id, actorID, roleID)
	if err != nil || lead == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, LeadNotFoundCode, "Lead not found")
		return
	}

	if err := h.Service.AssignOwner(id, req.AssigneeID, actorID, roleID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Failed to assign lead")
		return
	}
	updated, _ := h.Service.GetByID(id, actorID, roleID)
	c.JSON(http.StatusOK, updated)
}

// --- UpdateStatus ---
type updateLeadStatusRequest struct {
	To      string `json:"to" binding:"required"`
	Comment string `json:"comment"`
}

func (h *LeadHandler) UpdateStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}

	var req updateLeadStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}

	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	lead, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || lead == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, LeadNotFoundCode, "Lead not found")
		return
	}

	if req.To == "converted" {
		badRequest(c, "Use /leads/:id/convert for conversion")
		return
	}

	if err := h.Service.UpdateStatus(id, req.To, userID, roleID); err != nil {
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
			return
		}
		badRequest(c, "Failed to update lead status")
		return
	}

	updated, _ := h.Service.GetByID(id, userID, roleID)
	c.JSON(http.StatusOK, updated)
}

// --- Convert ---
type ConvertLeadRequest struct {
	Amount            float64 `json:"amount" example:"50000"`
	Currency          string  `json:"currency" example:"USD"`
	ClientName        string  `json:"client_name" binding:"required"`
	ClientBinIin      string  `json:"client_bin_iin"`
	ClientAddress     string  `json:"client_address"`
	ClientContactInfo string  `json:"client_contact_info"`
}

func (h *LeadHandler) ConvertToDeal(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}

	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}
	lead, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || lead == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, LeadNotFoundCode, "Lead not found")
		return
	}

	var req ConvertLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}

	client := &models.Client{
		Name:        req.ClientName,
		BinIin:      req.ClientBinIin,
		Address:     req.ClientAddress,
		ContactInfo: req.ClientContactInfo,
	}
	deal, convErr := h.Service.ConvertLeadToDeal(id, req.Amount, req.Currency, lead.OwnerID, userID, roleID, client)
	if convErr != nil {
		if errors.Is(convErr, services.ErrDealAlreadyExists) && deal != nil {
			c.JSON(http.StatusConflict, deal)
			return
		}
		conflict(c, ValidationFailed, "Lead conversion conflict")
		return
	}
	c.JSON(201, deal)
}

func (h *LeadHandler) List(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
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

	leads, err := h.Service.ListForRole(userID, roleID, size, offset)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Failed to list leads")
		return
	}
	c.JSON(http.StatusOK, leads)
}

// GET /leads/my?page=&size=
func (h *LeadHandler) ListMy(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
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

	leads, err := h.Service.ListMy(userID, size, offset)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Failed to list leads")
		return
	}
	c.JSON(http.StatusOK, leads)
}
