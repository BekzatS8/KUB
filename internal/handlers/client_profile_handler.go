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
	files := gin.H{}
	for _, category := range payload.PrimaryFileCatalog {
		exists := payload.PrimaryFileExists[category]
		files[category] = gin.H{
			"exists":       exists,
			"category":     category,
			"view_url":     base + "/files/primary?category=" + category,
			"download_url": base + "/files/primary/download?category=" + category,
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"client": payload.Client,
		"completeness": gin.H{
			"client_ref":       payload.ClientRef,
			"client_id":        payload.ClientRef.ClientID,
			"client_type":      payload.ClientRef.ClientType,
			"type":             payload.CompletenessType,
			"missing_yellow":   payload.MissingYellow,
			"yellow_ready":     len(payload.MissingYellow) == 0,
			"missing_contract": payload.MissingContract,
			"contract_ready":   payload.ContractReady,
			"contract_readiness": gin.H{
				"ready":   payload.ContractReady,
				"missing": payload.MissingContract,
			},
		},
		"files": files,
	})
}
