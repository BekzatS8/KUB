package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type CompanyHandler struct {
	service services.CompanyService
}

func NewCompanyHandler(service services.CompanyService) *CompanyHandler {
	return &CompanyHandler{service: service}
}

func (h *CompanyHandler) List(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	if userID <= 0 {
		unauthorized(c, "Unauthorized")
		return
	}

	memberships, err := h.service.ListUserCompanies(userID)
	if err != nil {
		internalError(c, "Failed to list companies")
		return
	}
	out := make([]*models.Company, 0, len(memberships))
	for _, m := range memberships {
		if m.Company == nil {
			continue
		}
		out = append(out, m.Company)
	}
	c.JSON(http.StatusOK, out)
}

func (h *CompanyHandler) GetByID(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	if userID <= 0 {
		unauthorized(c, "Unauthorized")
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		badRequest(c, "Invalid company ID")
		return
	}
	hasAccess, err := h.service.HasUserAccess(userID, id)
	if err != nil {
		internalError(c, "Failed to validate company access")
		return
	}
	if !hasAccess {
		notFound(c, "company_not_found", "Company not found")
		return
	}
	company, err := h.service.GetCompany(id)
	if err != nil {
		if err == sql.ErrNoRows {
			notFound(c, "company_not_found", "Company not found")
			return
		}
		internalError(c, "Failed to get company")
		return
	}
	c.JSON(http.StatusOK, company)
}

func (h *CompanyHandler) GetUserCompanies(c *gin.Context) {
	requesterID, requesterRole := getUserAndRole(c)
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil || userID <= 0 {
		badRequest(c, "Invalid user ID")
		return
	}

	if requesterID != userID && !(authz.CanViewLeadershipData(requesterRole) || authz.IsReadOnly(requesterRole)) {
		forbidden(c, "Forbidden")
		return
	}

	companies, err := h.service.ListUserCompanies(userID)
	if err != nil {
		internalError(c, "Failed to list user companies")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user_id":   userID,
		"companies": companies,
	})
}

func (h *CompanyHandler) GetMyCompanies(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	if userID <= 0 {
		unauthorized(c, "Unauthorized")
		return
	}

	companies, err := h.service.ListUserCompanies(userID)
	if err != nil {
		internalError(c, "Failed to list user companies")
		return
	}
	primaryCompanyID, _ := h.service.GetPrimaryCompanyID(userID)
	activeCompanyID, _ := h.service.GetUserActiveCompanyID(userID)

	c.JSON(http.StatusOK, gin.H{
		"user_id":            userID,
		"companies":          companies,
		"primary_company_id": primaryCompanyID,
		"active_company_id":  activeCompanyID,
	})
}

func (h *CompanyHandler) PutUserCompanies(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if roleID != authz.RoleSystemAdmin {
		forbidden(c, "Only system admin can update user companies")
		return
	}

	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil || userID <= 0 {
		badRequest(c, "Invalid user ID")
		return
	}

	var req struct {
		CompanyIDs       []int `json:"company_ids"`
		PrimaryCompanyID *int  `json:"primary_company_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}

	if err := h.service.ReplaceUserCompanies(userID, req.CompanyIDs, req.PrimaryCompanyID); err != nil {
		badRequest(c, err.Error())
		return
	}

	companies, err := h.service.ListUserCompanies(userID)
	if err != nil {
		internalError(c, "Failed to list user companies")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user_id":   userID,
		"companies": companies,
	})
}
