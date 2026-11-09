package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

// Карточка лида (ТЗ «доработки CRM КУБ», п.3): краткая информация, передача в
// отдел продаж, «Вернуть лид», объединение лидов, история.
type leadCardService interface {
	UpdateBrief(ctx context.Context, id int, brief models.LeadBrief, userID, roleID int) (*models.Leads, error)
	HandToSales(ctx context.Context, id, userID, roleID int) error
	ReturnLead(ctx context.Context, id int, reason string, userID, roleID int) error
	MergeLeads(ctx context.Context, targetID, sourceID, userID, roleID int) error
	History(ctx context.Context, id, userID, roleID int) ([]models.LeadHistoryEntry, error)
}

func (h *LeadHandler) cardService(c *gin.Context) (leadCardService, int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		badRequest(c, "Invalid id")
		return nil, 0, false
	}
	svc, ok := h.Service.(leadCardService)
	if !ok {
		internalError(c, "lead card is not supported")
		return nil, 0, false
	}
	return svc, id, true
}

func writeLeadCardError(c *gin.Context, err error, fallback string) {
	var briefErr *services.LeadBriefRequiredError
	switch {
	case errors.As(err, &briefErr):
		badRequestWithCode(c, "lead_brief_incomplete", "Заполните: "+strings.Join(briefErr.Missing, ", "))
	case errors.Is(err, services.ErrLeadReasonRequired):
		badRequestWithCode(c, "reason_required", "Укажите причину возврата")
	case errors.Is(err, services.ErrLeadAlreadyHanded):
		conflict(c, "lead_already_handed", "Лид уже передан в отдел продаж")
	case errors.Is(err, repositories.ErrLeadMergeConflict):
		badRequestWithCode(c, "lead_merge_conflict", "Эти лиды объединить нельзя")
	case errors.Is(err, services.ErrReadOnly):
		forbiddenWithCode(c, "lead_read_only", "Лид доступен только на просмотр")
	default:
		writeLeadOwnerError(c, err, fallback)
	}
}

func forbiddenWithCode(c *gin.Context, code, msg string) {
	writeError(c, http.StatusForbidden, code, msg)
}

// PUT /leads/:id/brief — «Краткая информация».
func (h *LeadHandler) UpdateBrief(c *gin.Context) {
	svc, id, ok := h.cardService(c)
	if !ok {
		return
	}
	var body models.LeadBrief
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	userID, roleID := getUserAndRole(c)
	lead, err := svc.UpdateBrief(c.Request.Context(), id, body, userID, roleID)
	if err != nil {
		writeLeadCardError(c, err, "Failed to update lead brief")
		return
	}
	c.JSON(http.StatusOK, lead)
}

// POST /leads/:id/hand-to-sales — «Передать в отдел продаж».
func (h *LeadHandler) HandToSales(c *gin.Context) {
	svc, id, ok := h.cardService(c)
	if !ok {
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := svc.HandToSales(c.Request.Context(), id, userID, roleID); err != nil {
		writeLeadCardError(c, err, "Failed to hand lead to sales")
		return
	}
	updated, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || updated == nil {
		c.JSON(http.StatusOK, gin.H{"id": id, "status": "handed"})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// POST /leads/:id/return {reason} — «Вернуть лид» в общую очередь.
func (h *LeadHandler) ReturnLead(c *gin.Context) {
	svc, id, ok := h.cardService(c)
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := svc.ReturnLead(c.Request.Context(), id, body.Reason, userID, roleID); err != nil {
		writeLeadCardError(c, err, "Failed to return lead")
		return
	}
	updated, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || updated == nil {
		c.JSON(http.StatusOK, gin.H{"id": id, "status": "returned"})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// POST /leads/:id/merge {lead_id} — влить лид lead_id в открытый лид :id.
func (h *LeadHandler) MergeLeads(c *gin.Context) {
	svc, id, ok := h.cardService(c)
	if !ok {
		return
	}
	var body struct {
		LeadID int `json:"lead_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.LeadID <= 0 {
		badRequest(c, "lead_id is required")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := svc.MergeLeads(c.Request.Context(), id, body.LeadID, userID, roleID); err != nil {
		writeLeadCardError(c, err, "Failed to merge leads")
		return
	}
	updated, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || updated == nil {
		c.JSON(http.StatusOK, gin.H{"id": id, "status": "merged"})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// GET /leads/:id/history — история лида.
func (h *LeadHandler) History(c *gin.Context) {
	svc, id, ok := h.cardService(c)
	if !ok {
		return
	}
	userID, roleID := getUserAndRole(c)
	items, err := svc.History(c.Request.Context(), id, userID, roleID)
	if err != nil {
		writeLeadCardError(c, err, "Failed to load lead history")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
