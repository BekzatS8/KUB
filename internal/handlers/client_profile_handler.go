package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"turcompany/internal/services"
)

type clientProfileProvider interface {
	GetProfile(ctx context.Context, clientID, userID, roleID int) (*services.ClientProfilePayload, error)
}

type ClientProfileHandler struct {
	Service clientProfileProvider
}

func NewClientProfileHandler(service clientProfileProvider) *ClientProfileHandler {
	return &ClientProfileHandler{Service: service}
}

func (h *ClientProfileHandler) GetProfile(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		badRequest(c, "Invalid client ID")
		return
	}
	userID, roleID := getUserAndRole(c)

	payload, err := h.Service.GetProfile(c.Request.Context(), id, userID, roleID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, ClientNotFoundCode, "Client not found")
		return
	}

	base := "/clients/" + strconv.Itoa(id)
	c.JSON(http.StatusOK, gin.H{
		"client": payload.Client,
		"completeness": gin.H{
			"client_id":      id,
			"missing_yellow": payload.MissingYellow,
			"yellow_ready":   len(payload.MissingYellow) == 0,
		},
		"files": gin.H{
			"photo35x45": gin.H{
				"exists":       payload.PhotoExists,
				"category":     "photo35x45",
				"view_url":     base + "/files/primary?category=photo35x45",
				"download_url": base + "/files/primary/download?category=photo35x45",
			},
		},
	})
}
