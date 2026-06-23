package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"turcompany/internal/services"
)

type UserApprovalHandler struct {
	svc *services.UserApprovalService
}

func NewUserApprovalHandler(svc *services.UserApprovalService) *UserApprovalHandler {
	return &UserApprovalHandler{svc: svc}
}

// List — GET /api/v1/user-requests?status=pending|all
// Только для админа (проверяется в роутере через RequireRoles).
func (h *UserApprovalHandler) List(c *gin.Context) {
	page, size := normalizedPageAndSize(c)
	limit, offset := size, offsetFromPage(page, size)

	statusFilter := c.DefaultQuery("status", "pending")
	var (
		items interface{}
		err   error
	)
	if statusFilter == "all" {
		items, err = h.svc.ListAll(c.Request.Context(), limit, offset)
	} else {
		items, err = h.svc.ListPending(c.Request.Context(), limit, offset)
	}
	if err != nil {
		internalError(c, "Не удалось получить список запросов")
		return
	}
	if items == nil {
		c.JSON(http.StatusOK, gin.H{"data": []struct{}{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

// ListMyRequests — GET /api/v1/user-requests/my
// Для юриста и HR: возвращает их собственные запросы (любой статус).
func (h *UserApprovalHandler) ListMyRequests(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	page, size := normalizedPageAndSize(c)
	limit, offset := size, offsetFromPage(page, size)

	items, err := h.svc.ListByRequester(c.Request.Context(), userID, limit, offset)
	if err != nil {
		internalError(c, "Не удалось получить список запросов")
		return
	}
	if items == nil {
		c.JSON(http.StatusOK, gin.H{"data": []struct{}{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

// Approve — POST /api/v1/user-requests/:id/approve
func (h *UserApprovalHandler) Approve(c *gin.Context) {
	reviewerID, _ := getUserAndRole(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Некорректный ID запроса")
		return
	}

	if err := h.svc.Approve(c.Request.Context(), id, reviewerID); err != nil {
		switch {
		case errors.Is(err, services.ErrApprovalNotFound):
			notFound(c, NotFoundCode, "Запрос не найден")
		case errors.Is(err, services.ErrApprovalAlreadyResolved):
			conflict(c, ConflictCode, "Запрос уже обработан")
		default:
			internalError(c, "Не удалось выполнить запрос")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Запрос одобрен и выполнен"})
}

// Reject — POST /api/v1/user-requests/:id/reject
func (h *UserApprovalHandler) Reject(c *gin.Context) {
	reviewerID, _ := getUserAndRole(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Некорректный ID запроса")
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)

	if err := h.svc.Reject(c.Request.Context(), id, reviewerID, body.Reason); err != nil {
		switch {
		case errors.Is(err, services.ErrApprovalNotFound):
			notFound(c, NotFoundCode, "Запрос не найден")
		case errors.Is(err, services.ErrApprovalAlreadyResolved):
			conflict(c, ConflictCode, "Запрос уже обработан")
		default:
			internalError(c, "Не удалось отклонить запрос")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Запрос отклонён"})
}
