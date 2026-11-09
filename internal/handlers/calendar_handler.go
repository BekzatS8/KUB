package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/models"
	"turcompany/internal/services"
)

// CalendarHandler — раздел «Календарь» (ТЗ «доработки CRM КУБ», п.6).
type CalendarHandler struct {
	svc *services.CalendarService
}

func NewCalendarHandler(svc *services.CalendarService) *CalendarHandler {
	return &CalendarHandler{svc: svc}
}

type calendarEventRequest struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	StartsAt    string  `json:"starts_at"` // RFC3339
	EndsAt      string  `json:"ends_at"`   // RFC3339, необязательно
	LeadID      *int    `json:"lead_id"`
	ClientID    *int    `json:"client_id"`
	OwnerID     *int    `json:"owner_id"`
}

func (r calendarEventRequest) toModel() (*models.CalendarEvent, error) {
	e := &models.CalendarEvent{
		Title:       r.Title,
		Description: r.Description,
		LeadID:      r.LeadID,
		ClientID:    r.ClientID,
		OwnerID:     r.OwnerID,
	}
	if strings.TrimSpace(r.StartsAt) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(r.StartsAt))
		if err != nil {
			return nil, errors.New("Неверная дата начала")
		}
		e.StartsAt = t
	}
	if strings.TrimSpace(r.EndsAt) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(r.EndsAt))
		if err != nil {
			return nil, errors.New("Неверная дата окончания")
		}
		e.EndsAt = &t
	}
	return e, nil
}

func writeCalendarError(c *gin.Context, err error, fallback string) {
	if msg, ok := services.IsCalendarValidation(err); ok {
		badRequestWithCode(c, "calendar_invalid", msg)
		return
	}
	switch {
	case errors.Is(err, services.ErrForbidden), errors.Is(err, services.ErrReadOnly):
		forbidden(c, "Forbidden")
	case errors.Is(err, services.ErrNotFound):
		notFound(c, "calendar_event_not_found", "Встреча не найдена")
	default:
		internalError(c, fallback)
	}
}

func optionalIntQuery(c *gin.Context, key string) *int {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return nil
	}
	return &v
}

// GET /calendar/events?from=RFC3339&to=RFC3339&owner_id=&branch_id=&lead_id=
func (h *CalendarHandler) List(c *gin.Context) {
	from, err1 := time.Parse(time.RFC3339, strings.TrimSpace(c.Query("from")))
	to, err2 := time.Parse(time.RFC3339, strings.TrimSpace(c.Query("to")))
	if err1 != nil || err2 != nil {
		badRequest(c, "from и to обязательны (RFC3339)")
		return
	}
	userID, roleID := getUserAndRole(c)
	items, err := h.svc.List(c.Request.Context(), models.CalendarEventFilter{
		From:     from,
		To:       to,
		OwnerID:  optionalIntQuery(c, "owner_id"),
		BranchID: optionalIntQuery(c, "branch_id"),
		LeadID:   optionalIntQuery(c, "lead_id"),
	}, userID, roleID)
	if err != nil {
		writeCalendarError(c, err, "Failed to list calendar events")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// POST /calendar/events
func (h *CalendarHandler) Create(c *gin.Context) {
	var req calendarEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	e, err := req.toModel()
	if err != nil {
		badRequestWithCode(c, "calendar_invalid", err.Error())
		return
	}
	userID, roleID := getUserAndRole(c)
	created, err := h.svc.Create(c.Request.Context(), e, userID, roleID)
	if err != nil {
		writeCalendarError(c, err, "Failed to create calendar event")
		return
	}
	c.JSON(http.StatusCreated, created)
}

// PUT /calendar/events/:id
func (h *CalendarHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		badRequest(c, "Invalid id")
		return
	}
	var req calendarEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	e, err := req.toModel()
	if err != nil {
		badRequestWithCode(c, "calendar_invalid", err.Error())
		return
	}
	e.ID = id
	userID, roleID := getUserAndRole(c)
	updated, err := h.svc.Update(c.Request.Context(), e, userID, roleID)
	if err != nil {
		writeCalendarError(c, err, "Failed to update calendar event")
		return
	}
	c.JSON(http.StatusOK, updated)
}

// DELETE /calendar/events/:id
func (h *CalendarHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		badRequest(c, "Invalid id")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.svc.Delete(c.Request.Context(), id, userID, roleID); err != nil {
		writeCalendarError(c, err, "Failed to delete calendar event")
		return
	}
	c.Status(http.StatusNoContent)
}
