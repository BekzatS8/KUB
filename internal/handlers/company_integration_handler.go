package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type CompanyIntegrationHandler struct {
	service services.CompanyIntegrationService
}

func NewCompanyIntegrationHandler(service services.CompanyIntegrationService) *CompanyIntegrationHandler {
	return &CompanyIntegrationHandler{service: service}
}

func (h *CompanyIntegrationHandler) List(c *gin.Context) {
	companyID, err := strconv.Atoi(c.Param("id"))
	if err != nil || companyID <= 0 {
		badRequest(c, "Invalid company id")
		return
	}
	userID, roleID := getUserAndRole(c)
	items, err := h.service.List(companyID, userID, roleID)
	if err != nil {
		if errors.Is(err, services.ErrIntegrationAccessDenied) {
			forbidden(c, "Forbidden")
			return
		}
		badRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *CompanyIntegrationHandler) Create(c *gin.Context) {
	companyID, err := strconv.Atoi(c.Param("id"))
	if err != nil || companyID <= 0 {
		badRequest(c, "Invalid company id")
		return
	}
	var in models.CompanyIntegration
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.service.Create(companyID, userID, roleID, &in); err != nil {
		if errors.Is(err, services.ErrIntegrationAccessDenied) {
			forbidden(c, "Forbidden")
			return
		}
		badRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusCreated, in)
}

func (h *CompanyIntegrationHandler) Update(c *gin.Context) {
	companyID, err := strconv.Atoi(c.Param("id"))
	if err != nil || companyID <= 0 {
		badRequest(c, "Invalid company id")
		return
	}
	integrationID, err := strconv.ParseInt(c.Param("integration_id"), 10, 64)
	if err != nil || integrationID <= 0 {
		badRequest(c, "Invalid integration id")
		return
	}
	var in models.CompanyIntegration
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.service.Update(companyID, integrationID, userID, roleID, &in); err != nil {
		if errors.Is(err, services.ErrIntegrationAccessDenied) {
			forbidden(c, "Forbidden")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			notFound(c, "integration_not_found", "Integration not found")
			return
		}
		badRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, in)
}

func (h *CompanyIntegrationHandler) Delete(c *gin.Context) {
	companyID, err := strconv.Atoi(c.Param("id"))
	if err != nil || companyID <= 0 {
		badRequest(c, "Invalid company id")
		return
	}
	integrationID, err := strconv.ParseInt(c.Param("integration_id"), 10, 64)
	if err != nil || integrationID <= 0 {
		badRequest(c, "Invalid integration id")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.service.Delete(companyID, integrationID, userID, roleID); err != nil {
		if errors.Is(err, services.ErrIntegrationAccessDenied) {
			forbidden(c, "Forbidden")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			notFound(c, "integration_not_found", "Integration not found")
			return
		}
		badRequest(c, err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}
