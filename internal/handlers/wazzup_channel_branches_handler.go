package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/repositories"
)

// Филиалы канала мессенджера: основной и дополнительные (миграция 099,
// замечания заказчика 10.11.2026 — Instagram kubvisa для KUB Шымкент и
// Алматы, visario.kz — для филиалов VISARIO).

// SetChannelBranchesRepo подключает хранилище дополнительных филиалов.
func (h *WazzupHandler) SetChannelBranchesRepo(r *repositories.ChannelBranchesRepository) {
	h.chBranches = r
}

// GET /integrations/wazzup/channel-branches — дополнительные филиалы по каналам.
func (h *WazzupHandler) ChannelBranches(c *gin.Context) {
	if h.chBranches == nil {
		c.JSON(http.StatusOK, gin.H{"items": map[string][]int{}})
		return
	}
	m, err := h.chBranches.ExtraBranches(c.Request.Context())
	if err != nil {
		internalError(c, "Failed to load channel branches")
		return
	}
	out := map[string][]int{}
	for ch, list := range m {
		out[strconv.FormatInt(ch, 10)] = list
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// PUT /integrations/wazzup/channels/:id/branches {branch_id, extra_branch_ids}
func (h *WazzupHandler) SetChannelBranches(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if roleID != authz.RoleSystemAdmin && roleID != authz.RoleManagement {
		forbidden(c, "Forbidden")
		return
	}
	if h.chBranches == nil {
		internalError(c, "channel branches unavailable")
		return
	}
	channelID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || channelID <= 0 {
		badRequest(c, "Invalid channel id")
		return
	}
	var req struct {
		BranchID       *int  `json:"branch_id"`
		ExtraBranchIDs []int `json:"extra_branch_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	if req.BranchID != nil && *req.BranchID <= 0 {
		req.BranchID = nil
	}
	// Без основного филиала дополнительные не имеют смысла: первый из них
	// становится основным.
	if req.BranchID == nil && len(req.ExtraBranchIDs) > 0 {
		first := req.ExtraBranchIDs[0]
		req.BranchID = &first
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	moved, err := h.chBranches.SetBranches(ctx, channelID, req.BranchID, req.ExtraBranchIDs)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			notFound(c, NotFoundCode, "Channel not found")
			return
		}
		log.Printf("[wazzup][channel_branches] channel=%d: %v", channelID, err)
		internalError(c, "Failed to set channel branches")
		return
	}
	log.Printf("[wazzup][channel_branches] channel=%d primary=%v extras=%v moved_leads=%d", channelID, req.BranchID, req.ExtraBranchIDs, moved)
	c.JSON(http.StatusOK, gin.H{"status": "ok", "moved_leads": moved})
}
