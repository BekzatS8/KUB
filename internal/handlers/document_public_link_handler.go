package handlers

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type DocumentPublicLinkHandler struct {
	Service *services.PublicDocumentSigningService
}

func NewDocumentPublicLinkHandler(service *services.PublicDocumentSigningService) *DocumentPublicLinkHandler {
	return &DocumentPublicLinkHandler{Service: service}
}

func (h *DocumentPublicLinkHandler) GenerateSignLink(c *gin.Context) {
	if h.Service == nil {
		internalError(c, "Service unavailable")
		return
	}
	documentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	var body struct {
		TTLMinutes int `json:"ttl_minutes"`
	}
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		badRequest(c, "Invalid request body")
		return
	}
	userID, roleID := getUserAndRole(c)
	url, expiresAt, err := h.Service.GenerateSignLink(c.Request.Context(), documentID, userID, roleID, body.TTLMinutes)
	if err != nil {
		switch {
		case errors.Is(err, repositories.ErrPublicLinkNotFound):
			notFound(c, DocumentNotFound, "Document not found")
		case errors.Is(err, services.ErrPublicSignInvalidStatus):
			conflict(c, InvalidStatusCode, "Invalid status")
		default:
			internalError(c, "Failed to generate sign link")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url, "expires_at": expiresAt})
}
