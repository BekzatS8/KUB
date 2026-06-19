package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"turcompany/internal/models"
	"turcompany/internal/services"
)

type FunnelTransitionRuleHandler struct {
	svc *services.FunnelTransitionRuleService
}

func NewFunnelTransitionRuleHandler(svc *services.FunnelTransitionRuleService) *FunnelTransitionRuleHandler {
	return &FunnelTransitionRuleHandler{svc: svc}
}

func (h *FunnelTransitionRuleHandler) List(c *gin.Context) {
	rules, err := h.svc.List()
	if err != nil {
		internalError(c, "Failed to list transition rules")
		return
	}
	if rules == nil {
		rules = []*models.FunnelTransitionRule{}
	}
	c.JSON(http.StatusOK, rules)
}

func (h *FunnelTransitionRuleHandler) Get(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	rule, err := h.svc.GetByID(id)
	if err != nil {
		if errors.Is(err, services.ErrNotFound) {
			notFound(c, ValidationFailed, "Rule not found")
			return
		}
		internalError(c, "Failed to get rule")
		return
	}
	c.JSON(http.StatusOK, rule)
}

func (h *FunnelTransitionRuleHandler) Create(c *gin.Context) {
	var rule models.FunnelTransitionRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	_, roleID := getUserAndRole(c)
	if err := h.svc.Create(&rule, roleID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		if errors.Is(err, services.ErrInvalidState) {
			badRequest(c, "Invalid stage/funnel combination")
			return
		}
		internalError(c, "Failed to create rule")
		return
	}
	c.JSON(http.StatusCreated, rule)
}

func (h *FunnelTransitionRuleHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	var rule models.FunnelTransitionRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	rule.ID = id
	_, roleID := getUserAndRole(c)
	if err := h.svc.Update(&rule, roleID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		if errors.Is(err, services.ErrNotFound) {
			notFound(c, ValidationFailed, "Rule not found")
			return
		}
		if errors.Is(err, services.ErrInvalidState) {
			badRequest(c, "Invalid stage/funnel combination")
			return
		}
		internalError(c, "Failed to update rule")
		return
	}
	c.JSON(http.StatusOK, rule)
}

func (h *FunnelTransitionRuleHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	_, roleID := getUserAndRole(c)
	if err := h.svc.Delete(id, roleID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		if errors.Is(err, services.ErrNotFound) {
			notFound(c, ValidationFailed, "Rule not found")
			return
		}
		internalError(c, "Failed to delete rule")
		return
	}
	c.Status(http.StatusNoContent)
}

type toggleActiveRequest struct {
	IsActive bool `json:"is_active"`
}

func (h *FunnelTransitionRuleHandler) ToggleActive(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	var req toggleActiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	_, roleID := getUserAndRole(c)
	if err := h.svc.ToggleActive(id, req.IsActive, roleID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "Forbidden")
			return
		}
		if errors.Is(err, services.ErrNotFound) {
			notFound(c, ValidationFailed, "Rule not found")
			return
		}
		internalError(c, "Failed to toggle rule")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "is_active": req.IsActive})
}
