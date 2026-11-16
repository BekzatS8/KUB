package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/repositories"
)

// Отделы для формы воронки и маршрут входящих лидов в колл-центр
// (замечания заказчика 13.11.2026).

// SetCRMSettings подключает настройки CRM.
func (h *FunnelHandler) SetCRMSettings(repo *repositories.CRMSettingsRepository) {
	h.settings = repo
}

// GET /funnels/departments — отделы, для которых можно завести воронку.
func (h *FunnelHandler) Departments(c *gin.Context) {
	if h.settings == nil {
		c.JSON(http.StatusOK, gin.H{"items": []repositories.FunnelDepartment{}})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	items, err := h.settings.FunnelDepartments(ctx)
	if err != nil {
		internalError(c, "Не удалось загрузить отделы")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func canManageInboundRouting(roleID int) bool {
	return roleID == authz.RoleSystemAdmin || roleID == authz.RoleManagement
}

// GET /funnels/inbound-routing — включён ли маршрут «входящие сначала в
// колл-центр» и сколько работающих операторов.
func (h *FunnelHandler) InboundRouting(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !canManageInboundRouting(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	if h.settings == nil {
		internalError(c, "Настройки недоступны")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	state, err := h.settings.InboundRouting(ctx, authz.RoleOperator)
	if err != nil {
		internalError(c, "Не удалось загрузить настройку")
		return
	}
	c.JSON(http.StatusOK, state)
}

// PUT /funnels/inbound-routing {to_callcenter: bool}
func (h *FunnelHandler) SetInboundRouting(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !canManageInboundRouting(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	if h.settings == nil {
		internalError(c, "Настройки недоступны")
		return
	}
	var req struct {
		ToCallcenter *bool `json:"to_callcenter"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ToCallcenter == nil {
		badRequest(c, "Укажите to_callcenter")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := h.settings.SetInboundToCallcenter(ctx, *req.ToCallcenter, userID); err != nil {
		log.Printf("[funnels] set inbound routing to_callcenter=%v by user=%d: %v", *req.ToCallcenter, userID, err)
		internalError(c, "Не удалось сохранить настройку")
		return
	}
	log.Printf("[funnels] inbound routing to_callcenter=%v by user=%d", *req.ToCallcenter, userID)
	state, err := h.settings.InboundRouting(ctx, authz.RoleOperator)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"to_callcenter": *req.ToCallcenter})
		return
	}
	c.JSON(http.StatusOK, state)
}
