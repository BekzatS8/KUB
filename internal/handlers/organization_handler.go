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

// publicOrgContactsDTO is the white-listed public view of organization contacts.
// Internal fields (id, bin, legal_name, address, created_at, updated_at) are excluded.
type publicOrgContactsDTO struct {
	Name      string `json:"name"`
	Phone     string `json:"phone,omitempty"`
	Email     string `json:"email,omitempty"`
	Website   string `json:"website,omitempty"`
	WhatsApp  string `json:"whatsapp,omitempty"`
	Telegram  string `json:"telegram,omitempty"`
	Instagram string `json:"instagram,omitempty"`
	TikTok    string `json:"tiktok,omitempty"`
	LogoURL   string `json:"logo_url,omitempty"`
}

// GetPublicContacts returns contact fields only — no JWT required.
// Intended for external websites. Sets Access-Control-Allow-Origin: * on the response
// so any origin can fetch without credentials.
func (h *OrganizationHandler) GetPublicContacts(c *gin.Context) {
	org, err := h.svc.Get()
	if err != nil {
		internalError(c, "Failed to load organization contacts")
		return
	}
	c.Header("Access-Control-Allow-Origin", "*")
	c.JSON(http.StatusOK, publicOrgContactsDTO{
		Name:      org.Name,
		Phone:     org.Phone,
		Email:     org.Email,
		Website:   org.Website,
		WhatsApp:  org.WhatsApp,
		Telegram:  org.Telegram,
		Instagram: org.Instagram,
		TikTok:    org.TikTok,
		LogoURL:   org.LogoURL,
	})
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
