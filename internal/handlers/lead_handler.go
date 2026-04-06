package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

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
	if authz.CanManageSystem(roleID) {
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
	if authz.CanManageSystem(roleID) {
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
	if authz.CanManageSystem(roleID) {
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
	if authz.CanManageSystem(roleID) {
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
	if authz.CanManageSystem(roleID) {
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
type ConvertLeadByIDRequest struct {
	Amount     float64 `json:"amount" binding:"required" example:"50000"`
	Currency   string  `json:"currency" binding:"required" example:"USD"`
	ClientID   int     `json:"client_id" binding:"required" example:"1"`
	ClientType string  `json:"client_type" binding:"required" example:"individual"`
}

type ConvertLeadWithClientRequest struct {
	Amount            float64 `json:"amount" binding:"required" example:"50000"`
	Currency          string  `json:"currency" binding:"required" example:"USD"`
	ClientType        string  `json:"client_type" binding:"required" example:"individual"`
	ClientName        string  `json:"client_name" binding:"required"`
	ClientBinIin      string  `json:"client_bin_iin"`
	ClientAddress     string  `json:"client_address"`
	ClientContactInfo string  `json:"client_contact_info"`
	Phone             string  `json:"phone"`
	Email             string  `json:"email"`

	CompanyName        string `json:"company_name"`
	Bin                string `json:"bin"`
	LegalAddress       string `json:"legal_address"`
	ContactPersonName  string `json:"contact_person_name"`
	ContactPersonPhone string `json:"contact_person_phone"`
	ContactPersonEmail string `json:"contact_person_email"`
}

func (h *LeadHandler) ConvertToDeal(c *gin.Context) {
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
	lead, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || lead == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, LeadNotFoundCode, "Lead not found")
		return
	}

	var req ConvertLeadByIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	if req.Amount <= 0 || strings.TrimSpace(req.Currency) == "" || req.ClientID <= 0 || strings.TrimSpace(req.ClientType) == "" {
		badRequest(c, "Invalid payload")
		return
	}

	deal, convErr := h.Service.ConvertLeadToDeal(id, req.Amount, req.Currency, lead.OwnerID, userID, roleID, req.ClientID, req.ClientType)
	if convErr != nil {
		if errors.Is(convErr, services.ErrClientNotFound) {
			notFound(c, ClientNotFoundCode, "Client not found")
			return
		}
		if errors.Is(convErr, services.ErrClientTypeRequired) || errors.Is(convErr, services.ErrClientTypeMismatch) {
			badRequest(c, convErr.Error())
			return
		}
		if strings.Contains(convErr.Error(), "invalid client_type") {
			badRequest(c, convErr.Error())
			return
		}
		if errors.Is(convErr, services.ErrDealAlreadyExists) && deal != nil {
			c.JSON(http.StatusConflict, deal)
			return
		}
		conflict(c, ValidationFailed, "Lead conversion conflict")
		return
	}
	c.JSON(201, deal)
}

func (h *LeadHandler) ConvertToDealWithClient(c *gin.Context) {
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
	lead, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || lead == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, LeadNotFoundCode, "Lead not found")
		return
	}

	var req ConvertLeadWithClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	if req.Amount <= 0 || strings.TrimSpace(req.Currency) == "" || strings.TrimSpace(req.ClientType) == "" {
		badRequest(c, "Invalid payload")
		return
	}
	client, mapErr := buildClientFromConvertWithClientRequest(req)
	if mapErr != nil {
		badRequest(c, "Invalid payload")
		return
	}
	deal, convErr := h.Service.ConvertLeadToDealWithClientData(id, req.Amount, req.Currency, lead.OwnerID, userID, roleID, client)
	if convErr != nil {
		if errors.Is(convErr, services.ErrClientTypeRequired) || errors.Is(convErr, services.ErrClientTypeMismatch) {
			badRequest(c, convErr.Error())
			return
		}
		if strings.Contains(convErr.Error(), "invalid client_type") {
			badRequest(c, convErr.Error())
			return
		}
		if errors.Is(convErr, services.ErrDealAlreadyExists) && deal != nil {
			c.JSON(http.StatusConflict, deal)
			return
		}
		conflict(c, ValidationFailed, "Lead conversion conflict")
		return
	}
	c.JSON(201, deal)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func buildClientFromConvertWithClientRequest(req ConvertLeadWithClientRequest) (*models.Client, error) {
	clientType := strings.ToLower(strings.TrimSpace(req.ClientType))
	if clientType == "" {
		return nil, services.ErrClientTypeRequired
	}
	client := &models.Client{ClientType: clientType, ContactInfo: req.ClientContactInfo}
	switch clientType {
	case models.ClientTypeLegal:
		client.Name = firstNonEmpty(req.CompanyName, req.ClientName)
		client.BinIin = firstNonEmpty(req.Bin, req.ClientBinIin)
		client.Address = firstNonEmpty(req.LegalAddress, req.ClientAddress)
		client.Phone = firstNonEmpty(req.ContactPersonPhone, req.Phone)
		client.Email = firstNonEmpty(req.ContactPersonEmail, req.Email)
		client.LegalProfile = &models.ClientLegalProfile{
			CompanyName:        firstNonEmpty(req.CompanyName, req.ClientName),
			BIN:                firstNonEmpty(req.Bin, req.ClientBinIin),
			LegalAddress:       firstNonEmpty(req.LegalAddress, req.ClientAddress),
			ContactPersonName:  strings.TrimSpace(req.ContactPersonName),
			ContactPersonPhone: firstNonEmpty(req.ContactPersonPhone, req.Phone),
			ContactPersonEmail: firstNonEmpty(req.ContactPersonEmail, req.Email),
		}
	case models.ClientTypeIndividual:
		client.Name = strings.TrimSpace(req.ClientName)
		client.BinIin = strings.TrimSpace(req.ClientBinIin)
		client.Address = strings.TrimSpace(req.ClientAddress)
		client.Phone = strings.TrimSpace(req.Phone)
		client.Email = strings.TrimSpace(req.Email)
	default:
		return nil, errors.New("invalid client_type")
	}
	if strings.TrimSpace(client.Name) == "" {
		return nil, errors.New("client name is required")
	}
	return client, nil
}

func (h *LeadHandler) List(c *gin.Context) {
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
