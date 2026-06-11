package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"turcompany/internal/models"
	"turcompany/internal/services"
)

type FunnelHandler struct {
	service *services.FunnelService
}

func NewFunnelHandler(service *services.FunnelService) *FunnelHandler {
	return &FunnelHandler{service: service}
}

func (h *FunnelHandler) List(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	funnels, err := h.service.List(userID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Failed to list funnels")
		return
	}
	c.JSON(http.StatusOK, funnels)
}

func (h *FunnelHandler) GetByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, _ := getUserAndRole(c)
	funnel, err := h.service.GetByID(id, userID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Failed to load funnel")
		return
	}
	if funnel == nil {
		notFound(c, ValidationFailed, "Funnel not found")
		return
	}
	c.JSON(http.StatusOK, funnel)
}

func (h *FunnelHandler) Create(c *gin.Context) {
	var req models.Funnel
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Code) == "" || req.DepartmentID <= 0 {
		badRequest(c, "Name, code and department_id are required")
		return
	}
	if req.SortOrder < 0 {
		req.SortOrder = 0
	}
	req.IsActive = true
	userID, _ := getUserAndRole(c)
	if err := h.service.Create(&req, userID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Failed to create funnel")
		return
	}
	c.JSON(http.StatusCreated, req)
}

func (h *FunnelHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	var req models.Funnel
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Code) == "" || req.DepartmentID <= 0 {
		badRequest(c, "Name, code and department_id are required")
		return
	}
	req.ID = id
	userID, _ := getUserAndRole(c)
	if err := h.service.Update(&req, userID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		if errors.Is(err, services.ErrNotFound) {
			notFound(c, ValidationFailed, "Funnel not found")
			return
		}
		internalError(c, "Failed to update funnel")
		return
	}
	updated, _ := h.service.GetByID(id, userID)
	c.JSON(http.StatusOK, updated)
}

func (h *FunnelHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, _ := getUserAndRole(c)
	if err := h.service.Delete(id, userID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		if errors.Is(err, services.ErrNotFound) {
			notFound(c, ValidationFailed, "Funnel not found")
			return
		}
		internalError(c, "Failed to delete funnel")
		return
	}
	c.Status(http.StatusNoContent)
}

type reorderFunnelsRequest struct {
	IDs []int `json:"ids"`
}

func (h *FunnelHandler) Reorder(c *gin.Context) {
	var req reorderFunnelsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		badRequest(c, "Invalid payload")
		return
	}
	userID, _ := getUserAndRole(c)
	if err := h.service.Reorder(req.IDs, userID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Failed to reorder funnels")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Funnels reordered"})
}

type moveLeadFunnelRequest struct {
	FunnelID int `json:"funnel_id" binding:"required"`
}

func (h *FunnelHandler) MoveLeadToFunnel(c *gin.Context) {
	leadID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	var req moveLeadFunnelRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.FunnelID <= 0 {
		badRequest(c, "Invalid payload")
		return
	}
	userID, _ := getUserAndRole(c)
	if err := h.service.MoveLeadToFunnel(leadID, req.FunnelID, userID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		if errors.Is(err, services.ErrNotFound) {
			notFound(c, ValidationFailed, "Lead or funnel not found")
			return
		}
		internalError(c, "Failed to move lead")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Lead moved"})
}
