package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"turcompany/internal/models"
	"turcompany/internal/services"
)

type FunnelStageHandler struct {
	service *services.FunnelStageService
}

func NewFunnelStageHandler(service *services.FunnelStageService) *FunnelStageHandler {
	return &FunnelStageHandler{service: service}
}

func (h *FunnelStageHandler) ListStages(c *gin.Context) {
	funnelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid funnel id")
		return
	}
	userID, _ := getUserAndRole(c)
	stages, err := h.service.ListStages(funnelID, userID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		if errors.Is(err, services.ErrNotFound) {
			notFound(c, ValidationFailed, "Funnel not found")
			return
		}
		internalError(c, "Failed to list stages")
		return
	}
	c.JSON(http.StatusOK, stages)
}

func (h *FunnelStageHandler) CreateStage(c *gin.Context) {
	funnelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid funnel id")
		return
	}
	var req models.FunnelStage
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	req.FunnelID = funnelID
	userID, _ := getUserAndRole(c)
	if err := h.service.CreateStage(&req, userID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		if errors.Is(err, services.ErrNotFound) {
			notFound(c, ValidationFailed, "Funnel not found")
			return
		}
		internalError(c, "Failed to create stage")
		return
	}
	c.JSON(http.StatusCreated, req)
}

func (h *FunnelStageHandler) UpdateStage(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	var req models.FunnelStage
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	req.ID = id
	userID, _ := getUserAndRole(c)
	if err := h.service.UpdateStage(&req, userID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		if errors.Is(err, services.ErrNotFound) {
			notFound(c, ValidationFailed, "Stage not found")
			return
		}
		internalError(c, "Failed to update stage")
		return
	}
	c.JSON(http.StatusOK, req)
}

type deleteStageRequest struct {
	ReassignToStageID *int `json:"reassign_to_stage_id"`
}

func (h *FunnelStageHandler) DeleteStage(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	var req deleteStageRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			badRequest(c, "Invalid payload")
			return
		}
	}
	if reassign := c.Query("reassign_to_stage_id"); reassign != "" && req.ReassignToStageID == nil {
		v, err := strconv.Atoi(reassign)
		if err != nil {
			badRequest(c, "Invalid reassign_to_stage_id")
			return
		}
		req.ReassignToStageID = &v
	}
	userID, _ := getUserAndRole(c)
	if err := h.service.DeleteStage(id, req.ReassignToStageID, userID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		if errors.Is(err, services.ErrNotFound) {
			notFound(c, ValidationFailed, "Stage not found")
			return
		}
		if errors.Is(err, services.ErrInvalidState) {
			badRequest(c, "Invalid reassign target")
			return
		}
		if errors.Is(err, services.ErrStageHasDeals) {
			conflict(c, ValidationFailed, "Stage has deals; reassign_to_stage_id is required")
			return
		}
		internalError(c, "Failed to delete stage")
		return
	}
	c.Status(http.StatusNoContent)
}

type reorderStagesRequest struct {
	IDs []int `json:"ids"`
}

func (h *FunnelStageHandler) ReorderStages(c *gin.Context) {
	funnelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid funnel id")
		return
	}
	var req reorderStagesRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		badRequest(c, "Invalid payload")
		return
	}
	userID, _ := getUserAndRole(c)
	if err := h.service.ReorderStages(funnelID, req.IDs, userID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		if errors.Is(err, services.ErrNotFound) {
			notFound(c, ValidationFailed, "Funnel not found")
			return
		}
		internalError(c, "Failed to reorder stages")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Stages reordered"})
}

func (h *FunnelStageHandler) DuplicateStage(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, _ := getUserAndRole(c)
	stage, err := h.service.DuplicateStage(id, userID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		if errors.Is(err, services.ErrNotFound) {
			notFound(c, ValidationFailed, "Stage not found")
			return
		}
		internalError(c, "Failed to duplicate stage")
		return
	}
	c.JSON(http.StatusCreated, stage)
}

func (h *FunnelStageHandler) Board(c *gin.Context) {
	funnelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid funnel id")
		return
	}
	userID, _ := getUserAndRole(c)
	board, err := h.service.Board(funnelID, userID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		if errors.Is(err, services.ErrNotFound) {
			notFound(c, ValidationFailed, "Funnel not found")
			return
		}
		internalError(c, "Failed to load board")
		return
	}
	c.JSON(http.StatusOK, board)
}
