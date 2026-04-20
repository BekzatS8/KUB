package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
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
		AssigneeID  int64               `json:"assignee_id"`
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
	if !authz.CanAccessTasks(roleID) {
		log.Printf("[task][create][deny] role=%d has no task access", roleID)
		forbidden(c, "Forbidden")
		return
	}
	if authz.IsReadOnly(roleID) {
		log.Printf("[task][create][deny] read-only role=%d", roleID)
		forbidden(c, "Read-only role")
		return
	}

	if req.AssigneeID == 0 {
		req.AssigneeID = uid
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
	if !canViewTask(roleID, int64(userID), task) || !h.hasTaskBranchAccess(roleID, int64(userID), task) {
		log.Printf("[task][getByID][deny] uid=%d role=%d", userID, roleID)
		forbidden(c, "Forbidden")
		return
	}
	log.Printf("[task][getByID][ok] id=%d", id)
	c.JSON(http.StatusOK, task)
}

// GET /tasks
func (h *TaskHandler) GetAll(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	log.Printf("[task][list] call by userID=%d role=%d q=%v", userID, roleID, c.Request.URL.RawQuery)

	if !authz.CanAccessTasks(roleID) {
		log.Printf("[task][list][deny] role=%d", roleID)
		forbidden(c, "Forbidden")
		return
	}

	filter, err := taskFilterFromQuery(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}

	uid := int64(userID)
	switch roleID {
	case authz.RoleSales:
		filter.AssigneeID = &uid
	case authz.RoleOperations:
		if me, err := h.users.GetByID(userID); err == nil && me != nil && me.BranchID != nil {
			b := int64(*me.BranchID)
			filter.BranchID = &b
		}
	case authz.RoleControl, authz.RoleManagement, authz.RoleSystemAdmin:
		// full or supervisory visibility — keep requested filter
	}

	if isPaginatedMode(c) {
		page, size := normalizedPageAndSize(c)
		offset := offsetFromPage(page, size)
		items, total, err := h.service.GetAllPaginated(c.Request.Context(), filter, size, offset)
		if err != nil {
			log.Printf("[task][list][err] %v", err)
			internalError(c, "Failed to retrieve tasks")
			return
		}
		log.Printf("[task][list][ok] count=%d total=%d", len(items), total)
		c.JSON(http.StatusOK, models.PaginatedResponse[models.Task]{Items: items, Pagination: buildPaginationMeta(page, size, total)})
		return
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

func taskFilterFromQuery(c *gin.Context) (models.TaskFilter, error) {
	filter := models.TaskFilter{
		Query:       strings.TrimSpace(c.Query("q")),
		StatusGroup: strings.ToLower(strings.TrimSpace(c.Query("status_group"))),
		SortBy:      strings.ToLower(strings.TrimSpace(c.Query("sort_by"))),
		Order:       strings.ToLower(strings.TrimSpace(c.Query("order"))),
		Archive:     strings.TrimSpace(c.Query("archive")),
	}
	if raw := strings.TrimSpace(c.Query("assignee_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return models.TaskFilter{}, errors.New("Invalid assignee_id")
		}
		filter.AssigneeID = &id
	}
	if raw := strings.TrimSpace(c.Query("creator_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return models.TaskFilter{}, errors.New("Invalid creator_id")
		}
		filter.CreatorID = &id
	}
	if raw := strings.TrimSpace(c.Query("entity_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return models.TaskFilter{}, errors.New("Invalid entity_id")
		}
		filter.EntityID = &id
	}
	if raw := strings.TrimSpace(c.Query("branch_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return models.TaskFilter{}, errors.New("Invalid branch_id")
		}
		filter.BranchID = &id
	}
	if v := strings.TrimSpace(c.Query("entity_type")); v != "" {
		filter.EntityType = &v
	}
	if v := strings.ToLower(strings.TrimSpace(c.Query("status"))); v != "" {
		st := models.TaskStatus(v)
		if !isAllowedTaskStatus(st) {
			return models.TaskFilter{}, errors.New("Invalid status")
		}
		filter.Status = &st
	}
	if filter.StatusGroup != "" && filter.StatusGroup != "active" && filter.StatusGroup != "closed" && filter.StatusGroup != "all" {
		return models.TaskFilter{}, errors.New("Invalid status_group")
	}
	if filter.SortBy != "" && filter.SortBy != "created_at" && filter.SortBy != "due_date" && filter.SortBy != "priority" && filter.SortBy != "status" && filter.SortBy != "title" {
		return models.TaskFilter{}, errors.New("Invalid sort_by")
	}
	if filter.Order != "" && filter.Order != "asc" && filter.Order != "desc" {
		return models.TaskFilter{}, errors.New("Invalid order")
	}
	return filter, nil
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

	if !canModifyTask(roleID, uid, current) || !h.hasTaskBranchAccess(roleID, uid, current) {
		log.Printf("[task][update][deny] uid=%d role=%d", uid, roleID)
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

	if !canModifyTask(roleID, uid, current) || !h.hasTaskBranchAccess(roleID, uid, current) {
		log.Printf("[task][delete][deny] uid=%d role=%d", uid, roleID)
		forbidden(c, "Forbidden")
		return
	}

	if !authz.CanHardDeleteBusinessEntity(roleID) {
		log.Printf("[task][delete][deny] hard-delete forbidden uid=%d role=%d", uid, roleID)
		forbidden(c, "Forbidden")
		return
	}

	if err := h.service.Delete(c.Request.Context(), id, uid, roleID); err != nil {
		if err == services.ErrForbidden {
			forbidden(c, "Forbidden")
			return
		}
		log.Printf("[task][delete][err] id=%d: %v", id, err)
		internalError(c, "Failed to delete task")
		return
	}

	log.Printf("[task][delete][ok] id=%d", id)

	// Телеграм-уведомление об удалении
	h.notifyAssignee(c, current, "🗑️ Задача удалена")

	c.Status(http.StatusNoContent)
}

type archiveTaskRequest struct {
	Reason string `json:"reason"`
}

func (h *TaskHandler) Archive(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}
	current, err := h.service.GetByIDWithArchiveScope(c.Request.Context(), id, repositories.ArchiveScopeAll)
	if err != nil {
		internalError(c, "Failed to get task")
		return
	}
	if current == nil {
		notFound(c, ValidationFailed, "Task not found")
		return
	}
	uid := int64(userID)
	if !canModifyTask(roleID, uid, current) || !h.hasTaskBranchAccess(roleID, uid, current) {
		forbidden(c, "Forbidden")
		return
	}

	var req archiveTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		badRequest(c, "Invalid payload")
		return
	}
	updated, err := h.service.ArchiveTask(c.Request.Context(), id, uid, roleID, req.Reason)
	if err != nil {
		if err == services.ErrForbidden || err == services.ErrReadOnly {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Failed to archive task")
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (h *TaskHandler) Unarchive(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	if authz.IsReadOnly(roleID) {
		forbidden(c, "Read-only role")
		return
	}
	current, err := h.service.GetByIDWithArchiveScope(c.Request.Context(), id, repositories.ArchiveScopeAll)
	if err != nil {
		internalError(c, "Failed to get task")
		return
	}
	if current == nil {
		notFound(c, ValidationFailed, "Task not found")
		return
	}
	uid := int64(userID)
	if !canModifyTask(roleID, uid, current) || !h.hasTaskBranchAccess(roleID, uid, current) {
		forbidden(c, "Forbidden")
		return
	}
	updated, err := h.service.UnarchiveTask(c.Request.Context(), id, uid, roleID)
	if err != nil {
		if err == services.ErrForbidden || err == services.ErrReadOnly {
			forbidden(c, "Forbidden")
			return
		}
		if err == services.ErrNotArchived {
			badRequest(c, "Task is not archived")
			return
		}
		internalError(c, "Failed to unarchive task")
		return
	}
	c.JSON(http.StatusOK, updated)
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

	if !canModifyTask(roleID, uid, current) || !h.hasTaskBranchAccess(roleID, uid, current) {
		log.Printf("[task][status][deny] uid=%d role=%d", uid, roleID)
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
	if body.To != models.StatusDone {
		h.notifyAssignee(c, updated, "🔁 Статус изменён на "+string(body.To))
	}
}

// POST /tasks/:id/complete
func (h *TaskHandler) Complete(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	log.Printf("[task][complete] call by userID=%d role=%d id_param=%s", userID, roleID, c.Param("id"))

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		log.Printf("[task][complete][err] invalid id: %v", err)
		badRequest(c, "Invalid id")
		return
	}
	uid := int64(userID)
	if authz.IsReadOnly(roleID) {
		log.Printf("[task][complete][deny] read-only role=%d", roleID)
		forbidden(c, "Read-only role")
		return
	}

	current, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		log.Printf("[task][complete][err] get current id=%d: %v", id, err)
		internalError(c, "Failed to get task")
		return
	}
	if current == nil {
		log.Printf("[task][complete][404] id=%d", id)
		notFound(c, ValidationFailed, "Task not found")
		return
	}
	if !canModifyTask(roleID, uid, current) || !h.hasTaskBranchAccess(roleID, uid, current) {
		log.Printf("[task][complete][deny] uid=%d role=%d", uid, roleID)
		forbidden(c, "Forbidden")
		return
	}

	updated, err := h.service.UpdateStatus(c.Request.Context(), id, models.StatusDone)
	if err != nil {
		log.Printf("[task][complete][err] save id=%d: %v", id, err)
		internalError(c, "Failed to complete task")
		return
	}
	log.Printf("[task][complete][ok] id=%d", id)
	c.JSON(http.StatusOK, updated)

}

// POST /tasks/:id/remind-later
func (h *TaskHandler) RemindLater(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	log.Printf("[task][remind] call by userID=%d role=%d id_param=%s", userID, roleID, c.Param("id"))

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		log.Printf("[task][remind][err] invalid id: %v", err)
		badRequest(c, "Invalid id")
		return
	}
	uid := int64(userID)
	if authz.IsReadOnly(roleID) {
		log.Printf("[task][remind][deny] read-only role=%d", roleID)
		forbidden(c, "Read-only role")
		return
	}

	current, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		log.Printf("[task][remind][err] get current id=%d: %v", id, err)
		internalError(c, "Failed to get task")
		return
	}
	if current == nil {
		log.Printf("[task][remind][404] id=%d", id)
		notFound(c, ValidationFailed, "Task not found")
		return
	}
	if !canModifyTask(roleID, uid, current) || !h.hasTaskBranchAccess(roleID, uid, current) {
		log.Printf("[task][remind][deny] uid=%d role=%d", uid, roleID)
		forbidden(c, "Forbidden")
		return
	}

	var body struct {
		Minutes    int    `json:"minutes"`
		ReminderAt string `json:"reminder_at"`
	}
	if err := c.ShouldBindJSON(&body); err != nil && err.Error() != "EOF" {
		log.Printf("[task][remind][bind][err] %v", err)
		badRequest(c, "Invalid payload")
		return
	}

	var newReminder time.Time
	if body.ReminderAt != "" {
		t, err := time.Parse(time.RFC3339, body.ReminderAt)
		if err != nil {
			log.Printf("[task][remind][err] invalid reminder_at=%q: %v", body.ReminderAt, err)
			badRequest(c, "Invalid reminder time")
			return
		}
		newReminder = t
	} else {
		minutes := body.Minutes
		if minutes <= 0 {
			minutes = 60
		}
		newReminder = time.Now().Add(time.Duration(minutes) * time.Minute)
	}

	update := *current
	update.ReminderAt = &newReminder
	update.UpdatedAt = time.Now()

	updated, err := h.service.Update(c.Request.Context(), id, &update)
	if err != nil {
		log.Printf("[task][remind][err] save id=%d: %v", id, err)
		internalError(c, "Failed to postpone reminder")
		return
	}
	log.Printf("[task][remind][ok] id=%d new=%s", id, newReminder.String())
	c.JSON(http.StatusOK, updated)
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

	if !canModifyTask(roleID, uid, current) || !h.hasTaskBranchAccess(roleID, uid, current) {
		log.Printf("[task][assign][deny] uid=%d role=%d", uid, roleID)
		forbidden(c, "Forbidden")
		return
	}
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

func canViewTask(roleID int, uid int64, t *models.Task) bool {
	switch roleID {
	case authz.RoleManagement, authz.RoleOperations, authz.RoleControl, authz.RoleSystemAdmin:
		return true
	case authz.RoleSales:
		return isOwnTask(uid, t)
	default:
		return false
	}
}

func canModifyTask(roleID int, uid int64, t *models.Task) bool {
	if authz.IsReadOnly(roleID) {
		return false
	}
	switch roleID {
	case authz.RoleManagement, authz.RoleOperations, authz.RoleSystemAdmin:
		return true
	case authz.RoleSales:
		return isOwnTask(uid, t)
	default:
		return false
	}
}

func isOwnTask(uid int64, t *models.Task) bool {
	if t == nil {
		return false
	}
	return t.CreatorID == uid || t.AssigneeID == uid
}

func (h *TaskHandler) hasTaskBranchAccess(roleID int, uid int64, t *models.Task) bool {
	if t == nil {
		return false
	}
	switch roleID {
	case authz.RoleManagement, authz.RoleSystemAdmin:
		return true
	case authz.RoleSales, authz.RoleOperations, authz.RoleControl:
		if h.users == nil || t.BranchID == nil {
			return false
		}
		me, err := h.users.GetByID(int(uid))
		if err != nil || me == nil || me.BranchID == nil {
			return false
		}
		return int64(*me.BranchID) == *t.BranchID
	default:
		return false
	}
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
	msg := prefix + "\n" + h.tg.FormatTaskNotification(t)
	if err := h.tg.SendMessage(chatID, msg); err != nil {
		log.Printf("[task][notify] send error: %v", err)
	}
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
	msg := "🗑️ Задача удалена\n" + h.tg.FormatTaskNotification(t)
	if err := h.tg.SendMessage(chatID, msg); err != nil {
		log.Printf("[task][notify][delete] send error: %v", err)
	}
}
