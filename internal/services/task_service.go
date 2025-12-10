// internal/services/tasks.go
package services

import (
	"context"
	"log"
	"time"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

// TaskService defines the interface for task-related business logic.
type TaskService interface {
	Create(ctx context.Context, task *models.Task) (*models.Task, error)
	GetByID(ctx context.Context, id int64) (*models.Task, error)
	GetAll(ctx context.Context, filter models.TaskFilter) ([]models.Task, error)
	Update(ctx context.Context, id int64, updateData *models.Task) (*models.Task, error)
	Delete(ctx context.Context, id int64) error

	// NEW:
	UpdateStatus(ctx context.Context, id int64, to models.TaskStatus) (*models.Task, error)
	UpdateAssignee(ctx context.Context, id int64, assigneeID int64) (*models.Task, error)
}

type taskService struct {
	repo  repositories.TaskRepository
	users repositories.UserRepository
	tg    *TelegramService
}

// NewTaskService creates a new instance of TaskService.
func NewTaskService(repo repositories.TaskRepository, users repositories.UserRepository, tg *TelegramService) TaskService {
	return &taskService{repo: repo, users: users, tg: tg}
}

func (s *taskService) Create(ctx context.Context, task *models.Task) (*models.Task, error) {
	if task.Status == "" {
		task.Status = models.StatusNew
	}
	if task.Priority == "" {
		task.Priority = models.PriorityNormal
	}
	now := time.Now()
	task.CreatedAt = now
	task.UpdatedAt = now
	if task.AssigneeID == 0 {
		task.AssigneeID = task.CreatorID
	}

	if err := s.repo.Store(ctx, task); err != nil {
		return nil, err
	}

	s.notifyTaskCreated(ctx, task)
	return task, nil
}

func (s *taskService) GetByID(ctx context.Context, id int64) (*models.Task, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *taskService) GetAll(ctx context.Context, filter models.TaskFilter) ([]models.Task, error) {
	return s.repo.FindAll(ctx, filter)
}

func (s *taskService) Update(ctx context.Context, id int64, updateData *models.Task) (*models.Task, error) {
	existingTask, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existingTask == nil {
		return nil, nil
	}

	// Прокидываем все поля, которые реально обновляет repo.Update
	existingTask.AssigneeID = updateData.AssigneeID
	existingTask.Title = updateData.Title
	existingTask.Description = updateData.Description
	existingTask.DueDate = updateData.DueDate
	existingTask.ReminderAt = updateData.ReminderAt
	existingTask.Priority = updateData.Priority
	existingTask.Status = updateData.Status

	existingTask.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, existingTask); err != nil {
		return nil, err
	}
	return existingTask, nil
}

func (s *taskService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func (s *taskService) UpdateStatus(ctx context.Context, id int64, to models.TaskStatus) (*models.Task, error) {
	// (валидацию переходов делает handler; сервис просто пишет)
	if err := s.repo.UpdateStatus(ctx, id, to); err != nil {
		return nil, err
	}
	updated, err := s.repo.FindByID(ctx, id)
	if err != nil || updated == nil {
		return updated, err
	}
	if to == models.StatusDone {
		s.notifyTaskCompleted(ctx, updated)
	}
	return updated, nil
}

func (s *taskService) UpdateAssignee(ctx context.Context, id int64, assigneeID int64) (*models.Task, error) {
	if err := s.repo.UpdateAssignee(ctx, id, assigneeID); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

const dueSoonThreshold = 24 * time.Hour

func (s *taskService) notifyTaskCreated(ctx context.Context, task *models.Task) {
	if s.tg == nil || s.users == nil || task == nil {
		return
	}
	if task.DueDate == nil {
		return
	}

	if d := time.Until(*task.DueDate); d < 0 || d > dueSoonThreshold {
		return
	}
	s.sendNotification(ctx, task, "📌 Новая задача")
}

func (s *taskService) notifyTaskCompleted(ctx context.Context, task *models.Task) {
	if s.tg == nil || s.users == nil || task == nil {
		return
	}
	s.sendNotification(ctx, task, "✅ Задача выполнена")
}

func (s *taskService) sendNotification(ctx context.Context, task *models.Task, prefix string) {
	if s.tg == nil || s.users == nil || task == nil {
		return
	}
	chatID, notify, err := s.users.GetTelegramSettings(ctx, task.AssigneeID)
	if err != nil {
		log.Printf("[task][notify] failed to get telegram settings for assignee=%d: %v", task.AssigneeID, err)
		return
	}
	if !notify || chatID == 0 {
		return
	}

	msg := prefix + "\n" + s.tg.FormatTaskNotification(task)
	if err := s.tg.SendMessage(chatID, msg); err != nil {
		log.Printf("[task][notify] telegram send error: %v", err)
	}
}
