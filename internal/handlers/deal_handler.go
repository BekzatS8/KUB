package handlers

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type DealHandler struct {
	Service dealService
}

type dealService interface {
	Create(deal *models.Deals, userID, roleID int) (int64, error)
	Update(deal *models.Deals, userID, roleID int) error
	GetByID(id int, userID, roleID int) (*models.Deals, error)
	Delete(id, userID, roleID int) error
	ListForRole(userID, roleID, limit, offset int, scope repositories.ArchiveScope, filter repositories.DealListFilter) ([]*models.Deals, error)
	ListMyWithFilterAndArchiveScope(ownerID, limit, offset int, scope repositories.ArchiveScope, filter repositories.DealListFilter) ([]*models.Deals, error)
	UpdateStatus(id int, to string, userID, roleID int) error
	ArchiveDeal(id, userID, roleID int, reason string) error
	UnarchiveDeal(id, userID, roleID int) error
	GetByIDWithArchiveScope(id int, userID, roleID int, scope repositories.ArchiveScope) (*models.Deals, error)
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
	if roleID == authz.RoleSales {
		deal.OwnerID = userID
	}
	if companyID, ok := GetActiveCompanyID(c); ok {
		deal.CompanyID = companyID
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
		if errors.Is(err, services.ErrLeadIDRequired) {
			badRequest(c, "Lead ID is required")
			return
		}
		if errors.Is(err, services.ErrLeadNotFound) {
			notFound(c, LeadNotFoundCode, "Lead not found")
			return
		}
		if errors.Is(err, services.ErrClientIDRequired) {
			badRequest(c, "Client ID is required")
			return
		}
		if errors.Is(err, services.ErrClientNotFound) {
			notFound(c, ClientNotFoundCode, "Client not found")
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
		if errors.Is(err, services.ErrInvalidClientType) {
			badRequest(c, "invalid client_type: allowed values are individual, legal")
			return
		}
		if errors.Is(err, services.ErrAmountInvalid) {
			badRequest(c, "Amount must be greater than 0")
			return
		}
		var dealConflict *services.DealAlreadyExistsError
		if errors.As(err, &dealConflict) {
			details := gin.H{"resource": "deal", "field": "lead_id", "value": dealConflict.LeadID}
			if dealConflict.ExistingDealID > 0 {
				details["existing_deal_id"] = dealConflict.ExistingDealID
			}
			writeErrorWithDetails(c, http.StatusConflict, DealAlreadyExistsCode, "Deal already exists for this lead", details)
			return
		}
		if errors.Is(err, services.ErrInvalidState) {
			conflict(c, InvalidStatusCode, "Invalid deal state")
			return
		}
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
			return
		}
		log.Printf("[DealHandler.Create] create failed: %v", err)
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

	current, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || current == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, DealNotFoundCode, "Deal not found")
		return
	}
	if companyID, ok := GetActiveCompanyID(c); ok && current.CompanyID != companyID {
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
		if errors.Is(err, services.ErrInvalidClientType) {
			badRequest(c, "invalid client_type: allowed values are individual, legal")
			return
		}
		if errors.Is(err, services.ErrAmountInvalid) {
			badRequest(c, "Amount must be greater than 0")
			return
		}
		var dealConflict *services.DealAlreadyExistsError
		if errors.As(err, &dealConflict) {
			details := gin.H{"resource": "deal", "field": "lead_id", "value": dealConflict.LeadID}
			if dealConflict.ExistingDealID > 0 {
				details["existing_deal_id"] = dealConflict.ExistingDealID
			}
			writeErrorWithDetails(c, http.StatusConflict, DealAlreadyExistsCode, "Deal already exists for this lead", details)
			return
		}
		if errors.Is(err, services.ErrLeadNotFound) {
			notFound(c, LeadNotFoundCode, "Lead not found")
			return
		}
		if errors.Is(err, services.ErrClientNotFound) {
			notFound(c, ClientNotFoundCode, "Client not found")
			return
		}
		if errors.Is(err, services.ErrInvalidState) {
			conflict(c, InvalidStatusCode, "Invalid deal state")
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
	deal, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || deal == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, DealNotFoundCode, "Deal not found")
		return
	}
	if companyID, ok := GetActiveCompanyID(c); ok && deal.CompanyID != companyID {
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
	if !authz.CanHardDeleteBusinessEntity(roleID) {
		forbidden(c, "Forbidden")
		return
	}

	deal, err := h.Service.GetByIDWithArchiveScope(id, userID, roleID, repositories.ArchiveScopeAll)
	if err != nil || deal == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, DealNotFoundCode, "Deal not found")
		return
	}
	if companyID, ok := GetActiveCompanyID(c); ok && deal.CompanyID != companyID {
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

type archiveDealRequest struct {
	Reason string `json:"reason"`
}

func (h *DealHandler) Archive(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	var req archiveDealRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		badRequest(c, "Invalid payload")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.Service.ArchiveDeal(id, userID, roleID, req.Reason); err != nil {
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
			return
		}
		if errors.Is(err, services.ErrDealNotFound) {
			notFound(c, DealNotFoundCode, "Deal not found")
			return
		}
		internalError(c, "Failed to archive deal")
		return
	}
	updated, _ := h.Service.GetByIDWithArchiveScope(id, userID, roleID, repositories.ArchiveScopeAll)
	c.JSON(http.StatusOK, updated)
}

func (h *DealHandler) Unarchive(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.Service.UnarchiveDeal(id, userID, roleID); err != nil {
		if errors.Is(err, services.ErrForbidden) || errors.Is(err, services.ErrReadOnly) {
			forbidden(c, err.Error())
			return
		}
		if errors.Is(err, services.ErrDealNotFound) {
			notFound(c, DealNotFoundCode, "Deal not found")
			return
		}
		if errors.Is(err, services.ErrNotArchived) {
			badRequest(c, "Deal is not archived")
			return
		}
		internalError(c, "Failed to unarchive deal")
		return
	}
	updated, _ := h.Service.GetByID(id, userID, roleID)
	c.JSON(http.StatusOK, updated)
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

	scope, ok := archiveScopeFromQuery(c)
	if !ok {
		badRequest(c, "Invalid archive filter")
		return
	}
	filter, err := dealListFilterFromQuery(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}

	deals, err := h.Service.ListForRole(userID, roleID, size, offset, scope, filter)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Failed to retrieve deals")
		return
	}
	if companyID, ok := GetActiveCompanyID(c); ok {
		deals = filterDealsByCompany(deals, companyID)
	}
	c.JSON(http.StatusOK, deals)
}

// GET /deals/my?page=&size=
func (h *DealHandler) ListMy(c *gin.Context) {
	userID, _ := getUserAndRole(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "100"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 100
	}
	offset := (page - 1) * size

	scope, ok := archiveScopeFromQuery(c)
	if !ok {
		badRequest(c, "Invalid archive filter")
		return
	}
	filter, err := dealListFilterFromQuery(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}

	deals, err := h.Service.ListMyWithFilterAndArchiveScope(userID, size, offset, scope, filter)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Failed to retrieve deals")
		return
	}
	if companyID, ok := GetActiveCompanyID(c); ok {
		deals = filterDealsByCompany(deals, companyID)
	}
	c.JSON(http.StatusOK, deals)
}

func filterDealsByCompany(items []*models.Deals, companyID int) []*models.Deals {
	if len(items) == 0 {
		return items
	}
	out := make([]*models.Deals, 0, len(items))
	for _, item := range items {
		if item == nil || item.CompanyID != companyID {
			continue
		}
		out = append(out, item)
	}
	return out
}

func dealListFilterFromQuery(c *gin.Context) (repositories.DealListFilter, error) {
	filter := repositories.DealListFilter{}
	clientIDRaw := strings.TrimSpace(c.Query("client_id"))
	if clientIDRaw != "" {
		clientID, err := strconv.Atoi(clientIDRaw)
		if err != nil || clientID <= 0 {
			return repositories.DealListFilter{}, errors.New("Invalid client_id")
		}
		filter.ClientID = clientID
	}
	filter.ClientType = strings.ToLower(strings.TrimSpace(c.Query("client_type")))
	filter.Query = strings.TrimSpace(c.Query("q"))
	filter.Status = strings.ToLower(strings.TrimSpace(c.Query("status")))
	if filter.Status != "" && !isAllowedDealStatus(filter.Status) {
		return repositories.DealListFilter{}, errors.New("Invalid status")
	}
	filter.StatusGroup = strings.ToLower(strings.TrimSpace(c.Query("status_group")))
	if filter.StatusGroup != "" && filter.StatusGroup != "active" && filter.StatusGroup != "completed" && filter.StatusGroup != "closed" && filter.StatusGroup != "all" {
		return repositories.DealListFilter{}, errors.New("Invalid status_group")
	}
	amountMinRaw := strings.TrimSpace(c.Query("amount_min"))
	if amountMinRaw != "" {
		amountMin, err := strconv.ParseFloat(amountMinRaw, 64)
		if err != nil {
			return repositories.DealListFilter{}, errors.New("Invalid amount_min")
		}
		filter.AmountMin = &amountMin
	}
	amountMaxRaw := strings.TrimSpace(c.Query("amount_max"))
	if amountMaxRaw != "" {
		amountMax, err := strconv.ParseFloat(amountMaxRaw, 64)
		if err != nil {
			return repositories.DealListFilter{}, errors.New("Invalid amount_max")
		}
		filter.AmountMax = &amountMax
	}
	filter.Currency = strings.ToUpper(strings.TrimSpace(c.Query("currency")))
	filter.SortBy = strings.ToLower(strings.TrimSpace(c.Query("sort_by")))
	if filter.SortBy != "" && filter.SortBy != "created_at" && filter.SortBy != "amount" && filter.SortBy != "status" && filter.SortBy != "client_name" {
		return repositories.DealListFilter{}, errors.New("Invalid sort_by")
	}
	filter.Order = strings.ToLower(strings.TrimSpace(c.Query("order")))
	if filter.Order != "" && filter.Order != "asc" && filter.Order != "desc" {
		return repositories.DealListFilter{}, errors.New("Invalid order")
	}
	return filter, nil
}

func isAllowedDealStatus(status string) bool {
	switch status {
	case "new", "in_progress", "negotiation", "won", "lost", "cancelled":
		return true
	default:
		return false
	}
}
