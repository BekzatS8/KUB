package services

import (
	"context"
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
}

type taskService struct {
	repo repositories.TaskRepository
}

// NewTaskService creates a new instance of TaskService.
func NewTaskService(repo repositories.TaskRepository) TaskService {
	return &taskService{repo: repo}
}

func (s *taskService) Create(ctx context.Context, task *models.Task) (*models.Task, error) {
	task.Status = models.StatusNew
	task.CreatedAt = time.Now()
	task.UpdatedAt = time.Now()

	if err := s.repo.Store(ctx, task); err != nil {
		return nil, err
	}
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

	// Update fields if they are provided in the request
	existingTask.AssigneeID = updateData.AssigneeID
	existingTask.Title = updateData.Title
	existingTask.Description = updateData.Description
	existingTask.DueDate = updateData.DueDate
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
