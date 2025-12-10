package handlers

import (
	"html"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type TaskHandler struct {
	service services.TaskService

	// ↓↓↓ Телеграм-уведомления
	tg    *services.TelegramService
	users repositories.UserRepository
}

func NewTaskHandler(service services.TaskService, tg *services.TelegramService, users repositories.UserRepository) *TaskHandler {
	return &TaskHandler{service: service, tg: tg, users: users}
}

// POST /tasks
func (h *TaskHandler) Create(c *gin.Context) {
	var req struct {
		AssigneeID  int64               `json:"assignee_id" binding:"required"`
		EntityID    int64               `json:"entity_id"`
		EntityType  string              `json:"entity_type"`
		Title       string              `json:"title" binding:"required"`
		Description string              `json:"description"`
		DueDate     string              `json:"due_date"`    // RFC3339
		ReminderAt  string              `json:"reminder_at"` // RFC3339
		Priority    models.TaskPriority `json:"priority"`    // low|normal|high|urgent
	}

	userID, roleID := getUserAndRole(c)
	log.Printf("[task][create] call by userID=%d role=%d", userID, roleID)

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[task][create][bind][err] %v", err)
		badRequest(c, "Invalid payload")
		return
	}
	log.Printf("[task][create] payload assignee_id=%d entity_type=%q entity_id=%d title=%q due=%q remind=%q priority=%q",
		req.AssigneeID, req.EntityType, req.EntityID, req.Title, req.DueDate, req.ReminderAt, req.Priority)

	uid := int64(userID)
	if authz.IsReadOnly(roleID) {
		log.Printf("[task][create][deny] read-only role=%d", roleID)
		forbidden(c, "Read-only role")
		return
	}

	if roleID == authz.RoleSales && req.AssigneeID != uid {
		log.Printf("[task][create][deny] staff=%d tried assign to %d", uid, req.AssigneeID)
		forbidden(c, "Staff can assign only to self")
		return
	}

	var due *time.Time
	if req.DueDate != "" {
		t, err := time.Parse(time.RFC3339, req.DueDate)
		if err != nil {
			log.Printf("[task][create][err] invalid due_date=%q: %v", req.DueDate, err)
			badRequest(c, "Invalid due date")
			return
		}
		due = &t
	}
	var rem *time.Time
	if req.ReminderAt != "" {
		t, err := time.Parse(time.RFC3339, req.ReminderAt)
		if err != nil {
			log.Printf("[task][create][err] invalid reminder_at=%q: %v", req.ReminderAt, err)
			badRequest(c, "Invalid reminder time")
			return
		}
		rem = &t
	}
	if req.Priority == "" {
		req.Priority = models.PriorityNormal
	}

	task := &models.Task{
		CreatorID:   uid,
		AssigneeID:  req.AssigneeID,
		EntityID:    req.EntityID,
		EntityType:  req.EntityType,
		Title:       req.Title,
		Description: req.Description,
		DueDate:     due,
		ReminderAt:  rem,
		Priority:    req.Priority,
	}

	createdTask, err := h.service.Create(c.Request.Context(), task)
	if err != nil {
		log.Printf("[task][create][err] %v", err)
		internalError(c, "Failed to create task")
		return
	}
	log.Printf("[task][create][ok] id=%d assignee_id=%d title=%q", createdTask.ID, createdTask.AssigneeID, createdTask.Title)
	c.JSON(http.StatusCreated, createdTask)

	// === TG: уведомление исполнителю ===
	h.notifyAssignee(c, createdTask, "📌 Новая задача")
}

// GET /tasks/:id
func (h *TaskHandler) GetByID(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	log.Printf("[task][getByID] call by userID=%d role=%d id_param=%s", userID, roleID, c.Param("id"))

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		log.Printf("[task][getByID][err] invalid id: %v", err)
		badRequest(c, "Invalid id")
		return
	}

	task, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		log.Printf("[task][getByID][err] id=%d: %v", id, err)
		internalError(c, "Failed to get task")
		return
	}
	if task == nil {
		log.Printf("[task][getByID][404] id=%d", id)
		notFound(c, ValidationFailed, "Task not found")
		return
	}
	log.Printf("[task][getByID][ok] id=%d", id)
	c.JSON(http.StatusOK, task)
}

// GET /tasks
func (h *TaskHandler) GetAll(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	log.Printf("[task][list] call by userID=%d role=%d q=%v", userID, roleID, c.Request.URL.RawQuery)

	var filter models.TaskFilter
	if v, ok := c.GetQuery("assignee_id"); ok {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.AssigneeID = &id
		} else {
			log.Printf("[task][list][warn] bad assignee_id=%q: %v", v, err)
		}
	}
	if v, ok := c.GetQuery("creator_id"); ok {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.CreatorID = &id
		} else {
			log.Printf("[task][list][warn] bad creator_id=%q: %v", v, err)
		}
	}
	if v, ok := c.GetQuery("entity_id"); ok {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.EntityID = &id
		} else {
			log.Printf("[task][list][warn] bad entity_id=%q: %v", v, err)
		}
	}
	if v, ok := c.GetQuery("entity_type"); ok {
		et := v
		filter.EntityType = &et
	}
	if v, ok := c.GetQuery("status"); ok {
		st := models.TaskStatus(v)
		filter.Status = &st
	}

	tasks, err := h.service.GetAll(c.Request.Context(), filter)
	if err != nil {
		log.Printf("[task][list][err] %v", err)
		internalError(c, "Failed to retrieve tasks")
		return
	}
	log.Printf("[task][list][ok] count=%d", len(tasks))
	c.JSON(http.StatusOK, tasks)
}

// PUT /tasks/:id
func (h *TaskHandler) Update(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	log.Printf("[task][update] call by userID=%d role=%d id_param=%s", userID, roleID, c.Param("id"))

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		log.Printf("[task][update][err] invalid id: %v", err)
		badRequest(c, "Invalid id")
		return
	}

	uid := int64(userID)
	if authz.IsReadOnly(roleID) {
		log.Printf("[task][update][deny] read-only role=%d", roleID)
		forbidden(c, "Read-only role")
		return
	}

	current, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		log.Printf("[task][update][err] get current id=%d: %v", id, err)
		internalError(c, "Failed to get task")
		return
	}
	if current == nil {
		log.Printf("[task][update][404] id=%d", id)
		notFound(c, ValidationFailed, "Task not found")
		return
	}

	if roleID == authz.RoleSales && !(current.CreatorID == uid || current.AssigneeID == uid) {
		log.Printf("[task][update][deny] staff uid=%d current creator=%d assignee=%d", uid, current.CreatorID, current.AssigneeID)
		forbidden(c, "Forbidden")
		return
	}

	var req struct {
		AssigneeID  *int64               `json:"assignee_id"`
		Title       *string              `json:"title"`
		Description *string              `json:"description"`
		DueDate     *string              `json:"due_date"`    // RFC3339
		ReminderAt  *string              `json:"reminder_at"` // RFC3339
		Priority    *models.TaskPriority `json:"priority"`
		Status      *models.TaskStatus   `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[task][update][bind][err] %v", err)
		badRequest(c, "Invalid payload")
		return
	}

	update := *current

	if req.AssigneeID != nil {
		if roleID == authz.RoleSales && *req.AssigneeID != uid {
			log.Printf("[task][update][deny] staff uid=%d set assignee=%d", uid, *req.AssigneeID)
			forbidden(c, "Staff can assign only to self")
			return
		}
		update.AssigneeID = *req.AssigneeID
	}
	if req.Title != nil {
		update.Title = *req.Title
	}
	if req.Description != nil {
		update.Description = *req.Description
	}
	if req.DueDate != nil {
		if *req.DueDate == "" {
			update.DueDate = nil
		} else {
			t, err := time.Parse(time.RFC3339, *req.DueDate)
			if err != nil {
				log.Printf("[task][update][err] invalid due_date=%q: %v", *req.DueDate, err)
				badRequest(c, "Invalid due date")
				return
			}
			update.DueDate = &t
		}
	}
	if req.ReminderAt != nil {
		if *req.ReminderAt == "" {
			update.ReminderAt = nil
		} else {
			t, err := time.Parse(time.RFC3339, *req.ReminderAt)
			if err != nil {
				log.Printf("[task][update][err] invalid reminder_at=%q: %v", *req.ReminderAt, err)
				badRequest(c, "Invalid reminder time")
				return
			}
			update.ReminderAt = &t
		}
	}
	if req.Priority != nil {
		update.Priority = *req.Priority
	}
	if req.Status != nil {
		if !isAllowedTaskStatus(*req.Status) || !isTransitionAllowed(current.Status, *req.Status) {
			log.Printf("[task][update][deny] illegal status transition: from=%q to=%q", current.Status, *req.Status)
			conflict(c, ValidationFailed, "Illegal status transition")
			return
		}
		update.Status = *req.Status
	}

	update.UpdatedAt = time.Now()

	updatedTask, err := h.service.Update(c.Request.Context(), id, &update)
	if err != nil {
		log.Printf("[task][update][err] save id=%d: %v", id, err)
		internalError(c, "Failed to update task")
		return
	}
	log.Printf("[task][update][ok] id=%d", id)
	c.JSON(http.StatusOK, updatedTask)

	// === TG: уведомление об обновлении ===
	h.notifyAssignee(c, updatedTask, "✏️ Задача обновлена")
}

// internal/handlers/task_handler.go

func (h *TaskHandler) Delete(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	log.Printf("[task][delete] call by userID=%d role=%d id_param=%s", userID, roleID, c.Param("id"))

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		log.Printf("[task][delete][err] invalid id: %v", err)
		badRequest(c, "Invalid id")
		return
	}

	uid := int64(userID)
	if authz.IsReadOnly(roleID) {
		log.Printf("[task][delete][deny] read-only role=%d", roleID)
		forbidden(c, "Read-only role")
		return
	}

	current, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		log.Printf("[task][delete][err] get current id=%d: %v", id, err)
		internalError(c, "Failed to get task")
		return
	}
	if current == nil {
		log.Printf("[task][delete][404] id=%d", id)
		notFound(c, ValidationFailed, "Task not found")
		return
	}

	if roleID == authz.RoleSales && current.CreatorID != uid {
		log.Printf("[task][delete][deny] staff uid=%d creator=%d", uid, current.CreatorID)
		forbidden(c, "Forbidden")
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		log.Printf("[task][delete][err] id=%d: %v", id, err)
		internalError(c, "Failed to delete task")
		return
	}

	log.Printf("[task][delete][ok] id=%d", id)

	// Телеграм-уведомление об удалении
	h.notifyAssignee(c, current, "🗑️ Задача удалена")

	c.Status(http.StatusNoContent)
}

// POST /tasks/:id/status { "to": "in_progress", "comment": "..." }
func (h *TaskHandler) ChangeStatus(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	log.Printf("[task][status] call by userID=%d role=%d id_param=%s", userID, roleID, c.Param("id"))

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		log.Printf("[task][status][err] invalid id: %v", err)
		badRequest(c, "Invalid id")
		return
	}

	uid := int64(userID)
	if authz.IsReadOnly(roleID) {
		log.Printf("[task][status][deny] read-only role=%d", roleID)
		forbidden(c, "Read-only role")
		return
	}

	current, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		log.Printf("[task][status][err] get current id=%d: %v", id, err)
		internalError(c, "Failed to get task")
		return
	}
	if current == nil {
		log.Printf("[task][status][404] id=%d", id)
		notFound(c, ValidationFailed, "Task not found")
		return
	}

	if roleID == authz.RoleSales && !(current.CreatorID == uid || current.AssigneeID == uid) {
		log.Printf("[task][status][deny] staff uid=%d creator=%d assignee=%d", uid, current.CreatorID, current.AssigneeID)
		forbidden(c, "Forbidden")
		return
	}

	var body struct {
		To      models.TaskStatus `json:"to" binding:"required"`
		Comment string            `json:"comment"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Printf("[task][status][bind][err] %v", err)
		badRequest(c, "Invalid payload")
		return
	}
	if !isAllowedTaskStatus(body.To) || !isTransitionAllowed(current.Status, body.To) {
		log.Printf("[task][status][deny] illegal transition from=%q to=%q", current.Status, body.To)
		conflict(c, ValidationFailed, "Illegal status")
		return
	}

	updated, err := h.service.UpdateStatus(c.Request.Context(), id, body.To)
	if err != nil {
		log.Printf("[task][status][err] save id=%d: %v", id, err)
		internalError(c, "Failed to update task status")
		return
	}
	log.Printf("[task][status][ok] id=%d new=%q", id, body.To)
	c.JSON(http.StatusOK, updated)

	// === TG: уведомление о смене статуса ===
	h.notifyAssignee(c, updated, "🔁 Статус изменён на "+string(body.To))
}

// POST /tasks/:id/assign { "assignee_id": 2, "comment":"..." }
func (h *TaskHandler) Assign(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	log.Printf("[task][assign] call by userID=%d role=%d id_param=%s", userID, roleID, c.Param("id"))

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		log.Printf("[task][assign][err] invalid id: %v", err)
		badRequest(c, "Invalid id")
		return
	}

	uid := int64(userID)
	if authz.IsReadOnly(roleID) {
		log.Printf("[task][assign][deny] read-only role=%d", roleID)
		forbidden(c, "Read-only role")
		return
	}

	current, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		log.Printf("[task][assign][err] get current id=%d: %v", id, err)
		internalError(c, "Failed to get task")
		return
	}
	if current == nil {
		log.Printf("[task][assign][404] id=%d", id)
		notFound(c, ValidationFailed, "Task not found")
		return
	}

	var body struct {
		AssigneeID int64  `json:"assignee_id" binding:"required"`
		Comment    string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Printf("[task][assign][bind][err] %v", err)
		badRequest(c, "Invalid payload")
		return
	}
	log.Printf("[task][assign] new_assignee=%d", body.AssigneeID)

	if roleID == authz.RoleSales && body.AssigneeID != uid {
		log.Printf("[task][assign][deny] staff uid=%d -> %d", uid, body.AssigneeID)
		forbidden(c, "Staff can assign only to self")
		return
	}

	updated, err := h.service.UpdateAssignee(c.Request.Context(), id, body.AssigneeID)
	if err != nil {
		log.Printf("[task][assign][err] save id=%d -> assignee=%d: %v", id, body.AssigneeID, err)
		internalError(c, "Failed to update assignee")
		return
	}
	log.Printf("[task][assign][ok] id=%d assignee=%d", id, body.AssigneeID)
	c.JSON(http.StatusOK, updated)

	// === TG: уведомление новому исполнителю ===
	h.notifyAssignee(c, updated, "👤 Вам назначена задача")
}

// ---- helpers ----
func isAllowedTaskStatus(s models.TaskStatus) bool {
	switch s {
	case models.StatusNew, models.StatusInProgress, models.StatusDone, models.StatusCancelled:
		return true
	}
	return false
}

func isTransitionAllowed(from, to models.TaskStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case models.StatusNew:
		return to == models.StatusInProgress || to == models.StatusCancelled
	case models.StatusInProgress:
		return to == models.StatusDone || to == models.StatusCancelled
	case models.StatusDone, models.StatusCancelled:
		return false
	}
	return false
}

// === TG helpers ===
func (h *TaskHandler) notifyAssignee(c *gin.Context, t *models.Task, prefix string) {
	if h.tg == nil || h.users == nil || t == nil {
		return
	}
	chatID, allow, err := h.users.GetTelegramSettings(c.Request.Context(), t.AssigneeID)
	if err != nil {
		log.Printf("[task][notify] get telegram settings failed: assignee=%d err=%v", t.AssigneeID, err)
		return
	}
	if !allow || chatID == 0 {
		log.Printf("[task][notify] skip: allow=%v chatID=%d", allow, chatID)
		return
	}
	_ = h.tg.SendMessage(chatID, h.formatTask(prefix, t))
}

// Лаконичное уведомление об удалении, без статуса/приоритета
func (h *TaskHandler) notifyAssigneeDeleted(c *gin.Context, t *models.Task) {
	if h.tg == nil || h.users == nil || t == nil {
		return
	}
	chatID, allow, err := h.users.GetTelegramSettings(c.Request.Context(), t.AssigneeID)
	if err != nil || !allow || chatID == 0 {
		return
	}
	due := "—"
	if t.DueDate != nil {
		due = t.DueDate.Format("2006-01-02 15:04")
	}
	msg := "🗑️ Задача удалена\n" +
		"• <b>" + html.EscapeString(t.Title) + "</b>\n" +
		"• Срок: <code>" + due + "</code>\n" +
		"• Связано: <code>" + t.EntityType + "#" + strconv.FormatInt(t.EntityID, 10) + "</code>"
	_ = h.tg.SendMessage(chatID, msg)
}

func (h *TaskHandler) formatTask(prefix string, t *models.Task) string {
	due := "—"
	if t.DueDate != nil {
		due = t.DueDate.Format("2006-01-02 15:04")
	}
	title := html.EscapeString(t.Title) // parse_mode=HTML
	return prefix + "\n" +
		"• <b>" + title + "</b>\n" +
		"• Статус: <code>" + string(t.Status) + "</code>\n" +
		"• Приоритет: <code>" + string(t.Priority) + "</code>\n" +
		"• Срок: <code>" + due + "</code>\n" +
		"• Связано: <code>" + t.EntityType + "#" + strconv.FormatInt(t.EntityID, 10) + "</code>"
}
