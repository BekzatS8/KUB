package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type FeedEventHandler struct {
	svc *services.FeedEventService
}

func NewFeedEventHandler(svc *services.FeedEventService) *FeedEventHandler {
	return &FeedEventHandler{svc: svc}
}

type createFeedEventRequest struct {
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	ResourceID *int            `json:"resource_id,omitempty"`
}

type rejectFeedEventRequest struct {
	Reason string `json:"reason"`
}

// POST /api/v1/feed-events
func (h *FeedEventHandler) Create(c *gin.Context) {
	userID, roleID := getUserAndRole(c)

	// Only roles with approvals.create can submit events for review.
	if !authz.HasPermission(authz.RoleCodeByID(roleID), "approvals.create") {
		forbidden(c, "У вас нет прав на отправку запросов")
		return
	}

	var req createFeedEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Некорректные данные запроса")
		return
	}

	validTypes := map[string]bool{
		models.FeedEventTypePendingCreateLead:   true,
		models.FeedEventTypePendingEditLead:     true,
		models.FeedEventTypePendingDeleteLead:   true,
		models.FeedEventTypePendingCreateDeal:   true,
		models.FeedEventTypePendingEditDeal:     true,
		models.FeedEventTypePendingDeleteDeal:   true,
		models.FeedEventTypePendingCreateClient:   true,
		models.FeedEventTypePendingEditClient:     true,
		models.FeedEventTypePendingDeleteClient:   true,
		models.FeedEventTypePendingCreateDocument: true,
		models.FeedEventTypePendingEditDocument:   true,
		models.FeedEventTypePendingDeleteDocument: true,
	}
	if !validTypes[req.Type] {
		badRequest(c, "Некорректный тип события")
		return
	}

	if len(req.Payload) == 0 {
		req.Payload = json.RawMessage("{}")
	}

	event, err := h.svc.Create(c.Request.Context(), userID, req.Type, req.Payload, req.ResourceID)
	if err != nil {
		internalError(c, "Не удалось создать запрос на подтверждение")
		return
	}
	c.JSON(http.StatusCreated, event)
}

// GET /api/v1/feed-events
func (h *FeedEventHandler) List(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	page, size := normalizedPageAndSize(c)
	limit, offset := size, offsetFromPage(page, size)

	status := c.DefaultQuery("status", "")

	events, err := h.svc.List(c.Request.Context(), userID, roleID, status, limit, offset)
	if err != nil {
		internalError(c, "Не удалось загрузить события")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": events, "total": len(events)})
}

// POST /api/v1/feed-events/:id/approve
func (h *FeedEventHandler) Approve(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if roleID != authz.RoleSystemAdmin {
		forbidden(c, "Только администратор может одобрять запросы")
		return
	}

	reviewerID, _ := getUserAndRole(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		badRequest(c, "Некорректный ID события")
		return
	}

	event, err := h.svc.Approve(c.Request.Context(), id, reviewerID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrFeedEventNotFound):
			notFound(c, NotFoundCode, "Событие не найдено")
		case errors.Is(err, services.ErrFeedEventAlreadyResolved):
			conflict(c, ConflictCode, "Запрос уже обработан")
		default:
			internalError(c, "Не удалось одобрить запрос: "+err.Error())
		}
		return
	}
	c.JSON(http.StatusOK, event)
}

// POST /api/v1/feed-events/:id/reject
func (h *FeedEventHandler) Reject(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if roleID != authz.RoleSystemAdmin {
		forbidden(c, "Только администратор может отклонять запросы")
		return
	}

	reviewerID, _ := getUserAndRole(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		badRequest(c, "Некорректный ID события")
		return
	}

	var req rejectFeedEventRequest
	_ = c.ShouldBindJSON(&req)

	event, err := h.svc.Reject(c.Request.Context(), id, reviewerID, req.Reason)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrFeedEventNotFound):
			notFound(c, NotFoundCode, "Событие не найдено")
		case errors.Is(err, services.ErrFeedEventAlreadyResolved):
			conflict(c, ConflictCode, "Запрос уже обработан")
		default:
			internalError(c, "Не удалось отклонить запрос")
		}
		return
	}
	c.JSON(http.StatusOK, event)
}
