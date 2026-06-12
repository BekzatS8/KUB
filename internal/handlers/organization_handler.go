package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type OrganizationHandler struct {
	svc services.OrganizationService
}

func NewOrganizationHandler(svc services.OrganizationService) *OrganizationHandler {
	return &OrganizationHandler{svc: svc}
}

// Get returns the singleton organization row.
// Accessible to all authenticated users (contacts are not secret).
func (h *OrganizationHandler) Get(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !authz.IsKnownRole(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	org, err := h.svc.Get()
	if err != nil {
		internalError(c, "Failed to load organization")
		return
	}
	c.JSON(http.StatusOK, org)
}

// Update replaces non-nil fields on the singleton.
// Only RoleSystemAdmin may write.
func (h *OrganizationHandler) Update(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if roleID != authz.RoleSystemAdmin {
		forbidden(c, "Only system admin can update organization settings")
		return
	}
	var req models.UpdateOrganizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	org, err := h.svc.Update(&req)
	if err != nil {
		internalError(c, "Failed to update organization")
		return
	}
	c.JSON(http.StatusOK, org)
}
