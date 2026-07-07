package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/repositories"
)

// ManagerReportHandler — личные отчёты сотрудников: редактируемая таблица
// внутри CRM вместо Excel на Яндекс.Диске (ТЗ 04.07.2026, п.3).
type ManagerReportHandler struct {
	repo *repositories.ManagerReportRepository
}

func NewManagerReportHandler(repo *repositories.ManagerReportRepository) *ManagerReportHandler {
	return &ManagerReportHandler{repo: repo}
}

// defaultReportContent — стартовая структура таблицы: те же колонки, что
// менеджеры вели в Excel (дата, имя, телефон, комментарий, статус).
var defaultReportContent = json.RawMessage(`{
	"columns": ["Дата", "Имя", "Телефон", "Комментарий", "Статус"],
	"rows": []
}`)

// GetMy — GET /reports/table/my: отчёт текущего пользователя
// (пустой шаблон, если ещё не создан).
func (h *ManagerReportHandler) GetMy(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	rep, err := h.repo.GetByUser(c.Request.Context(), userID)
	if err != nil {
		internalError(c, "Failed to load report")
		return
	}
	if rep == nil {
		c.JSON(http.StatusOK, gin.H{"user_id": userID, "content": defaultReportContent, "updated_at": nil})
		return
	}
	c.JSON(http.StatusOK, rep)
}

type saveReportRequest struct {
	Content json.RawMessage `json:"content" binding:"required"`
}

// SaveMy — PUT /reports/table/my: сохранить свою таблицу.
func (h *ManagerReportHandler) SaveMy(c *gin.Context) {
	// свой отчёт ведут все бизнес-роли, включая контроль качества (read-only
	// роль в остальной системе) — ТЗ п.3
	userID, _ := getUserAndRole(c)
	var req saveReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	if len(req.Content) > 2<<20 {
		badRequest(c, "Report is too large")
		return
	}
	if !json.Valid(req.Content) {
		badRequest(c, "Invalid content JSON")
		return
	}
	if err := h.repo.Upsert(c.Request.Context(), userID, req.Content); err != nil {
		internalError(c, "Failed to save report")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// List — GET /reports/table: список отчётов сотрудников
// (руководство/админ/КК: кто ведёт отчёт, когда обновлял).
func (h *ManagerReportHandler) List(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !authz.CanViewAllBusinessData(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	reports, err := h.repo.List(c.Request.Context())
	if err != nil {
		internalError(c, "Failed to list reports")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": reports, "count": len(reports)})
}

// GetByUser — GET /reports/table/user/:id: отчёт сотрудника
// (руководство/админ/КК, только просмотр).
func (h *ManagerReportHandler) GetByUser(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !authz.CanViewAllBusinessData(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	targetID, err := strconv.Atoi(c.Param("id"))
	if err != nil || targetID <= 0 {
		badRequest(c, "Invalid user id")
		return
	}
	rep, err := h.repo.GetByUser(c.Request.Context(), targetID)
	if err != nil {
		internalError(c, "Failed to load report")
		return
	}
	if rep == nil {
		c.JSON(http.StatusOK, gin.H{"user_id": targetID, "content": defaultReportContent, "updated_at": nil})
		return
	}
	c.JSON(http.StatusOK, rep)
}
