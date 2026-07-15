package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/repositories"
)

// ManagerReportHandler — личные отчёты сотрудников: редактируемые таблицы
// внутри CRM вместо Excel на Яндекс.Диске (ТЗ 04.07.2026, п.3).
// У сотрудника может быть несколько именованных отчётов; руководитель/КК
// выбирают, какой именно отчёт сотрудника открыть.
type ManagerReportHandler struct {
	repo *repositories.ManagerReportRepository
}

func NewManagerReportHandler(repo *repositories.ManagerReportRepository) *ManagerReportHandler {
	return &ManagerReportHandler{repo: repo}
}

// defaultReportContent — стартовая структура таблицы: те же колонки, что
// менеджеры вели в Excel (дата, имя, телефон, комментарий, тип визы).
var defaultReportContent = json.RawMessage(`{
	"columns": ["Дата", "Имя", "Телефон", "Тип визы", "Комментарий"],
	"rows": []
}`)

const (
	maxReportContentBytes = 2 << 20
	maxReportTitleLen     = 120
	defaultReportTitle    = "Новый отчёт"
)

// normalizeTitle приводит имя отчёта к безопасному виду: без пустых строк и
// без «простыни» вместо названия.
func normalizeTitle(raw string) string {
	title := strings.TrimSpace(raw)
	if title == "" {
		return defaultReportTitle
	}
	if len([]rune(title)) > maxReportTitleLen {
		title = string([]rune(title)[:maxReportTitleLen])
	}
	return title
}

// validateContent проверяет размер и валидность JSON таблицы.
func validateContent(c *gin.Context, content json.RawMessage) bool {
	if len(content) > maxReportContentBytes {
		badRequest(c, "Report is too large")
		return false
	}
	if !json.Valid(content) {
		badRequest(c, "Invalid content JSON")
		return false
	}
	return true
}

// ListMy — GET /reports/table/my: мои отчёты (без содержимого).
func (h *ManagerReportHandler) ListMy(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	reports, err := h.repo.ListByUser(c.Request.Context(), userID)
	if err != nil {
		internalError(c, "Failed to load reports")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": reports, "count": len(reports)})
}

type createReportRequest struct {
	Title   string          `json:"title"`
	Content json.RawMessage `json:"content"`
}

// CreateMy — POST /reports/table/my: завести новый отчёт с названием.
func (h *ManagerReportHandler) CreateMy(c *gin.Context) {
	// свои отчёты ведут все бизнес-роли, включая контроль качества (read-only
	// роль в остальной системе) — ТЗ п.3
	userID, _ := getUserAndRole(c)
	var req createReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	content := req.Content
	if len(content) == 0 {
		content = defaultReportContent
	}
	if !validateContent(c, content) {
		return
	}
	rep, err := h.repo.Create(c.Request.Context(), userID, normalizeTitle(req.Title), content)
	if err != nil {
		internalError(c, "Failed to create report")
		return
	}
	c.JSON(http.StatusCreated, rep)
}

// GetMy — GET /reports/table/my/:id: один мой отчёт с содержимым.
func (h *ManagerReportHandler) GetMy(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	rep, ok := h.loadOwned(c, userID)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, rep)
}

type saveReportRequest struct {
	Title   string          `json:"title"`
	Content json.RawMessage `json:"content" binding:"required"`
}

// SaveMy — PUT /reports/table/my/:id: сохранить свой отчёт (таблицу и название).
func (h *ManagerReportHandler) SaveMy(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	reportID, ok := parseReportID(c)
	if !ok {
		return
	}
	var req saveReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	if !validateContent(c, req.Content) {
		return
	}
	// пустое title в запросе не должно затирать название: берём текущее
	title := strings.TrimSpace(req.Title)
	if title == "" {
		existing, err := h.repo.GetByID(c.Request.Context(), reportID)
		if err != nil {
			internalError(c, "Failed to save report")
			return
		}
		if existing == nil || existing.UserID != userID {
			notFound(c, NotFoundCode, "Отчёт не найден")
			return
		}
		title = existing.Title
	}
	updated, err := h.repo.Update(c.Request.Context(), reportID, userID, normalizeTitle(title), req.Content)
	if err != nil {
		internalError(c, "Failed to save report")
		return
	}
	if !updated {
		notFound(c, NotFoundCode, "Отчёт не найден")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// DeleteMy — DELETE /reports/table/my/:id: удалить свой отчёт.
func (h *ManagerReportHandler) DeleteMy(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	reportID, ok := parseReportID(c)
	if !ok {
		return
	}
	deleted, err := h.repo.Delete(c.Request.Context(), reportID, userID)
	if err != nil {
		internalError(c, "Failed to delete report")
		return
	}
	if !deleted {
		notFound(c, NotFoundCode, "Отчёт не найден")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// List — GET /reports/table: сотрудники, которые ведут отчёты
// (руководство/админ/КК: кто, сколько отчётов, когда обновлял).
func (h *ManagerReportHandler) List(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !authz.CanViewAllBusinessData(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	owners, err := h.repo.ListOwners(c.Request.Context())
	if err != nil {
		internalError(c, "Failed to list reports")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": owners, "count": len(owners)})
}

// ListByUser — GET /reports/table/user/:id: отчёты сотрудника без содержимого
// (руководство/админ/КК выбирают, какой открыть).
func (h *ManagerReportHandler) ListByUser(c *gin.Context) {
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
	reports, err := h.repo.ListByUser(c.Request.Context(), targetID)
	if err != nil {
		internalError(c, "Failed to load reports")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": reports, "count": len(reports)})
}

// GetReport — GET /reports/table/report/:id: любой отчёт с содержимым
// (руководство/админ/КК, только просмотр).
func (h *ManagerReportHandler) GetReport(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !authz.CanViewAllBusinessData(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	reportID, ok := parseReportID(c)
	if !ok {
		return
	}
	rep, err := h.repo.GetByID(c.Request.Context(), reportID)
	if err != nil {
		internalError(c, "Failed to load report")
		return
	}
	if rep == nil {
		notFound(c, NotFoundCode, "Отчёт не найден")
		return
	}
	c.JSON(http.StatusOK, rep)
}

func parseReportID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		badRequest(c, "Invalid report id")
		return 0, false
	}
	return id, true
}

// loadOwned читает отчёт и убеждается, что он принадлежит userID: чужой отчёт
// на своём эндпоинте неотличим от несуществующего.
func (h *ManagerReportHandler) loadOwned(c *gin.Context, userID int) (*repositories.ManagerReport, bool) {
	reportID, ok := parseReportID(c)
	if !ok {
		return nil, false
	}
	rep, err := h.repo.GetByID(c.Request.Context(), reportID)
	if err != nil {
		internalError(c, "Failed to load report")
		return nil, false
	}
	if rep == nil || rep.UserID != userID {
		notFound(c, NotFoundCode, "Отчёт не найден")
		return nil, false
	}
	return rep, true
}
