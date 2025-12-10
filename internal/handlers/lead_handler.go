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
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	lead.OwnerID = userID
	if lead.Status == "" {
		lead.Status = "new"
	}
	if lead.CreatedAt.IsZero() {
		lead.CreatedAt = time.Now()
	}

	if err := h.Service.Create(&lead); err != nil {
		internalError(c, "Failed to create lead")
		return
	}
	c.JSON(http.StatusCreated, lead)
}

func (h *LeadHandler) Update(c *gin.Context) {
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
		notFound(c, LeadNotFoundCode, "Lead not found")
		return
	}
	if current.OwnerID != userID && !authz.IsElevated(roleID) {
		forbidden(c, "Forbidden")
		return
	}

	var body models.Leads
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	body.ID = id
	if !authz.IsElevated(roleID) {
		body.OwnerID = current.OwnerID
	}

	if err := h.Service.Update(&body); err != nil {
		internalError(c, "Failed to update lead")
		return
	}
	updated, _ := h.Service.GetByID(id)
	c.JSON(200, updated)
}

func (h *LeadHandler) GetByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}

	userID, roleID := getUserAndRole(c)
	lead, err := h.Service.GetByID(id)
	if err != nil || lead == nil {
		notFound(c, LeadNotFoundCode, "Lead not found")
		return
	}
	if lead.OwnerID != userID && !authz.IsElevated(roleID) && roleID != authz.RoleAudit {
		forbidden(c, "Forbidden")
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
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	lead, err := h.Service.GetByID(id)
	if err != nil || lead == nil {
		notFound(c, LeadNotFoundCode, "Lead not found")
		return
	}
	if lead.OwnerID != userID && !authz.IsElevated(roleID) {
		forbidden(c, "Forbidden")
		return
	}

	if err := h.Service.Delete(id); err != nil {
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
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	lead, err := h.Service.GetByID(id)
	if err != nil || lead == nil {
		notFound(c, LeadNotFoundCode, "Lead not found")
		return
	}

	if !authz.IsElevated(roleID) && req.AssigneeID != actorID {
		forbidden(c, "Only self-assign allowed")
		return
	}

	if err := h.Service.AssignOwner(id, req.AssigneeID); err != nil {
		internalError(c, "Failed to assign lead")
		return
	}
	updated, _ := h.Service.GetByID(id)
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
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}

	lead, err := h.Service.GetByID(id)
	if err != nil || lead == nil {
		notFound(c, LeadNotFoundCode, "Lead not found")
		return
	}

	if lead.OwnerID != userID && !authz.IsElevated(roleID) {
		forbidden(c, "Forbidden")
		return
	}

	if req.To == "converted" {
		badRequest(c, "Use /leads/:id/convert for conversion")
		return
	}

	if err := h.Service.UpdateStatus(id, req.To); err != nil {
		badRequest(c, "Failed to update lead status")
		return
	}

	updated, _ := h.Service.GetByID(id)
	c.JSON(http.StatusOK, updated)
}

// --- Convert ---
type ConvertLeadRequest struct {
	Amount            string `json:"amount" example:"50000"`
	Currency          string `json:"currency" example:"USD"`
	ClientName        string `json:"client_name" binding:"required"`
	ClientBinIin      string `json:"client_bin_iin"`
	ClientAddress     string `json:"client_address"`
	ClientContactInfo string `json:"client_contact_info"`
}

func (h *LeadHandler) ConvertToDeal(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}

	userID, roleID := getUserAndRole(c)
	lead, err := h.Service.GetByID(id)
	if err != nil || lead == nil {
		notFound(c, LeadNotFoundCode, "Lead not found")
		return
	}
	if lead.OwnerID != userID && !authz.IsElevated(roleID) {
		forbidden(c, "Forbidden")
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
	deal, convErr := h.Service.ConvertLeadToDeal(id, req.Amount, req.Currency, lead.OwnerID, client)
	if convErr != nil {
		conflict(c, ValidationFailed, "Lead conversion conflict")
		return
	}
	c.JSON(201, deal)
}

func (h *LeadHandler) List(c *gin.Context) {
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
	var leads []*models.Leads
	var err error

	if authz.IsElevated(roleID) || roleID == authz.RoleAudit {
		leads, err = h.Service.ListPaginated(size, offset)
	} else {
		leads, err = h.Service.ListMy(userID, size, offset)
	}
	if err != nil {
		internalError(c, "Failed to list leads")
		return
	}
	c.JSON(http.StatusOK, leads)
}
