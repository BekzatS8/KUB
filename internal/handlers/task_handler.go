package handlers

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"time"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

// @Tags tasks

// TaskHandler handles HTTP requests for tasks.
type TaskHandler struct {
	service services.TaskService
}

// NewTaskHandler creates a new TaskHandler.
func NewTaskHandler(service services.TaskService) *TaskHandler {
	return &TaskHandler{service: service}
}

func (h *TaskHandler) Create(c *gin.Context) {
	var req struct {
		AssigneeID  int64  `json:"assignee_id" binding:"required"`
		EntityID    int64  `json:"entity_id"`
		EntityType  string `json:"entity_type"`
		Title       string `json:"title" binding:"required"`
		Description string `json:"description"`
		DueDate     string `json:"due_date"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Assume creatorID is set by auth middleware
	// creatorID := c.MustGet("userID").(int64)
	creatorID := int64(1) // Placeholder

	var dueDate time.Time
	if req.DueDate != "" {
		parsedDate, err := time.Parse(time.RFC3339, req.DueDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid due_date format, use RFC3339"})
			return
		}
		dueDate = parsedDate
	}

	task := &models.Task{
		CreatorID:   creatorID,
		AssigneeID:  req.AssigneeID,
		EntityID:    req.EntityID,
		EntityType:  req.EntityType,
		Title:       req.Title,
		Description: req.Description,
		DueDate:     &dueDate,
	}

	createdTask, err := h.service.Create(c.Request.Context(), task)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
		return
	}
	c.JSON(http.StatusCreated, createdTask)
}

func (h *TaskHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID format"})
		return
	}

	task, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	c.JSON(http.StatusOK, task)
}
func (h *TaskHandler) GetAll(c *gin.Context) {
	var filter models.TaskFilter

	if assigneeIDStr, ok := c.GetQuery("assignee_id"); ok {
		if id, err := strconv.ParseInt(assigneeIDStr, 10, 64); err == nil {
			filter.AssigneeID = &id
		}
	}

	tasks, err := h.service.GetAll(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve tasks"})
		return
	}
	c.JSON(http.StatusOK, tasks)
}

func (h *TaskHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID format"})
		return
	}

	var req models.Task
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updatedTask, err := h.service.Update(c.Request.Context(), id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updatedTask)
}

func (h *TaskHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID format"})
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
