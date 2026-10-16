package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/authz"
	wz "turcompany/internal/integrations/wazzup"
)

// Доступ сотрудников к чатам номера Wazzup (окно «Доступ к чатам» на странице
// каналов). Только админ и руководство — те же, кто привязывает номера к
// филиалам.

type setChannelRolesRequest struct {
	Items []wz.ChannelRoleInput `json:"items"`
}

func (h *WazzupHandler) channelRolesPrecheck(c *gin.Context) (int64, bool) {
	_, roleID := getUserAndRole(c)
	if roleID != authz.RoleSystemAdmin && roleID != authz.RoleManagement {
		forbidden(c, "Forbidden")
		return 0, false
	}
	channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || channelID <= 0 {
		badRequest(c, "Invalid channel id")
		return 0, false
	}
	return channelID, true
}

func writeChannelRolesError(c *gin.Context, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, NotFoundCode, "Channel not found")
		return
	}
	writeWazzupError(c, err, "failed to update channel access")
}

// GET /integrations/wazzup/channels/:id/roles
func (h *WazzupHandler) ChannelRoles(c *gin.Context) {
	channelID, ok := h.channelRolesPrecheck(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	view, err := h.svc.ChannelRoles(ctx, channelID)
	if err != nil {
		writeChannelRolesError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// PUT /integrations/wazzup/channels/:id/roles — сохранить доступ вручную и
// сразу отправить роли в Wazzup.
func (h *WazzupHandler) SetChannelRoles(c *gin.Context) {
	channelID, ok := h.channelRolesPrecheck(c)
	if !ok {
		return
	}
	var req setChannelRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	view, err := h.svc.SetChannelRoles(ctx, channelID, req.Items)
	if err != nil {
		writeChannelRolesError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// DELETE /integrations/wazzup/channels/:id/roles — вернуть автоматические роли.
func (h *WazzupHandler) ResetChannelRoles(c *gin.Context) {
	channelID, ok := h.channelRolesPrecheck(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	view, err := h.svc.ResetChannelRoles(ctx, channelID)
	if err != nil {
		writeChannelRolesError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}
