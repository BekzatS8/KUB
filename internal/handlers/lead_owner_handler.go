package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"turcompany/internal/services"
)

// Ответственный за лид: «Взять себе» и список, кому можно передать лид.
type leadOwnerService interface {
	TakeLead(id, userID, roleID int) error
	Assignees(id, roleID int) ([]services.LeadAssignee, error)
}

// writeLeadOwnerError — общие ответы для назначения/«Взять себе».
func writeLeadOwnerError(c *gin.Context, err error, fallback string) {
	var assignErr *services.LeadAssignError
	switch {
	case errors.As(err, &assignErr):
		badRequestWithCode(c, "lead_assignee_invalid", assignErr.Msg)
	case errors.Is(err, services.ErrLeadTaken):
		conflict(c, "lead_taken", "Лид уже в работе у другого сотрудника — обновите страницу")
	case errors.Is(err, services.ErrForbidden), errors.Is(err, services.ErrReadOnly):
		forbidden(c, "Forbidden")
	case errors.Is(err, services.ErrLeadNotFound):
		notFound(c, LeadNotFoundCode, "Lead not found")
	default:
		internalError(c, fallback)
	}
}

// POST /leads/:id/take — сотрудник берёт себе лид без ответственного.
func (h *LeadHandler) Take(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	svc, ok := h.Service.(leadOwnerService)
	if !ok {
		internalError(c, "lead take is not supported")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := svc.TakeLead(id, userID, roleID); err != nil {
		writeLeadOwnerError(c, err, "Failed to take lead")
		return
	}
	updated, err := h.Service.GetByID(id, userID, roleID)
	if err != nil || updated == nil {
		c.JSON(http.StatusOK, gin.H{"id": id, "owner_id": userID})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// GET /leads/:id/assignees — кому можно передать лид (админ и руководство).
func (h *LeadHandler) Assignees(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	svc, ok := h.Service.(leadOwnerService)
	if !ok {
		internalError(c, "lead assignees are not supported")
		return
	}
	_, roleID := getUserAndRole(c)
	items, err := svc.Assignees(id, roleID)
	if err != nil {
		writeLeadOwnerError(c, err, "Failed to list assignees")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// dealOwnerService — «Взять себе» для сделки без ответственного.
type dealOwnerService interface {
	TakeDeal(id, userID, roleID int) error
}

// POST /deals/:id/take — сотрудник берёт себе ничью сделку своего филиала.
func (h *DealHandler) Take(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	svc, ok := h.Service.(dealOwnerService)
	if !ok {
		internalError(c, "deal take is not supported")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := svc.TakeDeal(id, userID, roleID); err != nil {
		if errors.Is(err, services.ErrDealNotFound) {
			notFound(c, DealNotFoundCode, "Deal not found")
			return
		}
		if errors.Is(err, services.ErrLeadTaken) {
			conflict(c, "deal_taken", "Сделка уже в работе у другого сотрудника — обновите страницу")
			return
		}
		writeLeadOwnerError(c, err, "Failed to take deal")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "owner_id": userID})
}
