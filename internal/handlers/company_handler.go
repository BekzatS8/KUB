package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"turcompany/internal/authz"
	"turcompany/internal/services"
)

type CompanyHandler struct {
	service services.CompanyService
}

func NewCompanyHandler(service services.CompanyService) *CompanyHandler {
	return &CompanyHandler{service: service}
}

func (h *CompanyHandler) List(c *gin.Context) {
	companies, err := h.service.ListCompanies()
	if err != nil {
		internalError(c, "Failed to list companies")
		return
	}
	c.JSON(http.StatusOK, companies)
}

func (h *CompanyHandler) GetByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		badRequest(c, "Invalid company ID")
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
