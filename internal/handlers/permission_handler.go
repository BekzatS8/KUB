package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"turcompany/internal/services"
)

type PermissionHandler struct {
	service *services.PermissionService
}

func NewPermissionHandler(service *services.PermissionService) *PermissionHandler {
	return &PermissionHandler{service: service}
}

func (h *PermissionHandler) GetMe(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	resp, err := h.service.GetMe(userID)
	if err != nil {
		if errors.Is(err, services.ErrNotFound) {
			notFound(c, ValidationFailed, "User not found")
			return
		}
		internalError(c, "Failed to load permissions")
		return
	}
	c.JSON(http.StatusOK, resp)
}
