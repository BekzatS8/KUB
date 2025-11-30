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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, roleID := getUserAndRole(c)
	if authz.IsReadOnly(roleID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "read-only role"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, lead)
}

func (h *LeadHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}

	userID, roleID := getUserAndRole(c)
	if authz.IsReadOnly(roleID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "read-only role"})
		return
	}

	current, err := h.Service.GetByID(id)
	if err != nil || current == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "lead not found"})
		return
	}
	if current.OwnerID != userID && !authz.IsElevated(roleID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	var body models.Leads
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	body.ID = id
	if !authz.IsElevated(roleID) {
		body.OwnerID = current.OwnerID
	}

	if err := h.Service.Update(&body); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	updated, _ := h.Service.GetByID(id)
	c.JSON(200, updated)
}

func (h *LeadHandler) GetByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}

	userID, roleID := getUserAndRole(c)
	lead, err := h.Service.GetByID(id)
	if err != nil || lead == nil {
		c.JSON(404, gin.H{"error": "lead not found"})
		return
	}
	if lead.OwnerID != userID && !authz.IsElevated(roleID) && roleID != authz.RoleAudit {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}
	c.JSON(200, lead)
}

func (h *LeadHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}

	userID, roleID := getUserAndRole(c)
	if authz.IsReadOnly(roleID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "read-only role"})
		return
	}

	lead, err := h.Service.GetByID(id)
	if err != nil || lead == nil {
		c.JSON(404, gin.H{"error": "lead not found"})
		return
	}
	if lead.OwnerID != userID && !authz.IsElevated(roleID) {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}

	if err := h.Service.Delete(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req assignLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	actorID, roleID := getUserAndRole(c)
	if authz.IsReadOnly(roleID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "read-only role"})
		return
	}

	lead, err := h.Service.GetByID(id)
	if err != nil || lead == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "lead not found"})
		return
	}

	if !authz.IsElevated(roleID) && req.AssigneeID != actorID {
		c.JSON(http.StatusForbidden, gin.H{"error": "only self-assign allowed"})
		return
	}

	if err := h.Service.AssignOwner(id, req.AssigneeID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req updateLeadStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, roleID := getUserAndRole(c)
	if authz.IsReadOnly(roleID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "read-only role"})
		return
	}

	lead, err := h.Service.GetByID(id)
	if err != nil || lead == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "lead not found"})
		return
	}

	if lead.OwnerID != userID && !authz.IsElevated(roleID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	if req.To == "converted" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "use /leads/:id/convert for conversion"})
		return
	}

	if err := h.Service.UpdateStatus(id, req.To); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}

	userID, roleID := getUserAndRole(c)
	lead, err := h.Service.GetByID(id)
	if err != nil || lead == nil {
		c.JSON(404, gin.H{"error": "lead not found"})
		return
	}
	if lead.OwnerID != userID && !authz.IsElevated(roleID) {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}

	var req ConvertLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
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
		c.JSON(409, gin.H{"error": convErr.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list leads"})
		return
	}
	c.JSON(http.StatusOK, leads)
}
