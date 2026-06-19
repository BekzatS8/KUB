package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"turcompany/internal/services"
)

type FeedHandler struct {
	audit *services.AuditService
}

func NewFeedHandler(audit *services.AuditService) *FeedHandler {
	return &FeedHandler{audit: audit}
}

func (h *FeedHandler) List(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	page, size := normalizedPageAndSize(c)
	limit, offset := size, offsetFromPage(page, size)

	entries, err := h.audit.ListFeed(c.Request.Context(), userID, roleID, limit, offset)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "forbidden")
			return
		}
		internalError(c, "failed to load feed")
		return
	}

	if entries == nil {
		entries = []*services.FeedEntry{}
	}
	c.JSON(http.StatusOK, gin.H{"data": entries})
}
