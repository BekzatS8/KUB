package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type taskBranchServiceStub struct {
	task             *models.Task
	updateStatusCall int
}

func (s *taskBranchServiceStub) Create(context.Context, *models.Task) (*models.Task, error) { return nil, nil }
func (s *taskBranchServiceStub) GetByID(context.Context, int64) (*models.Task, error)        { return s.task, nil }
func (s *taskBranchServiceStub) GetByIDWithArchiveScope(context.Context, int64, repositories.ArchiveScope) (*models.Task, error) {
	return s.task, nil
}
func (s *taskBranchServiceStub) GetAll(context.Context, models.TaskFilter) ([]models.Task, error) {
	return nil, nil
}
func (s *taskBranchServiceStub) GetAllPaginated(context.Context, models.TaskFilter, int, int) ([]models.Task, int, error) {
	return nil, 0, nil
}
func (s *taskBranchServiceStub) Update(context.Context, int64, *models.Task) (*models.Task, error) {
	return s.task, nil
}
func (s *taskBranchServiceStub) Delete(context.Context, int64, int64, int) error { return nil }
func (s *taskBranchServiceStub) ArchiveTask(context.Context, int64, int64, int, string) (*models.Task, error) {
	return s.task, nil
}
func (s *taskBranchServiceStub) UnarchiveTask(context.Context, int64, int64, int) (*models.Task, error) {
	return s.task, nil
}
func (s *taskBranchServiceStub) UpdateStatus(context.Context, int64, models.TaskStatus) (*models.Task, error) {
	s.updateStatusCall++
	return s.task, nil
}
func (s *taskBranchServiceStub) UpdateAssignee(context.Context, int64, int64) (*models.Task, error) {
	return s.task, nil
}

type taskBranchUserRepoStub struct {
	users map[int]*models.User
}

func (r *taskBranchUserRepoStub) Create(*models.User) error { return nil }
func (r *taskBranchUserRepoStub) GetByID(id int) (*models.User, error) {
	if u, ok := r.users[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, nil
}
func (r *taskBranchUserRepoStub) Update(*models.User) error                         { return nil }
func (r *taskBranchUserRepoStub) Delete(int) error                                  { return nil }
func (r *taskBranchUserRepoStub) List(int, int) ([]*models.User, error)             { return nil, nil }
func (r *taskBranchUserRepoStub) GetByEmail(string) (*models.User, error)           { return nil, nil }
func (r *taskBranchUserRepoStub) GetCount() (int, error)                            { return 0, nil }
func (r *taskBranchUserRepoStub) GetCountByRole(int) (int, error)                   { return 0, nil }
func (r *taskBranchUserRepoStub) UpdatePassword(int, string) error                  { return nil }
func (r *taskBranchUserRepoStub) UpdateRefresh(int, string, time.Time) error        { return nil }
func (r *taskBranchUserRepoStub) RotateRefresh(string, string, time.Time) (*models.User, error) {
	return nil, nil
}
func (r *taskBranchUserRepoStub) ClearRefresh(int) error                            { return nil }
func (r *taskBranchUserRepoStub) GetByRefreshToken(string) (*models.User, error)   { return nil, nil }
func (r *taskBranchUserRepoStub) VerifyUser(int) error                              { return nil }
func (r *taskBranchUserRepoStub) UpdateTelegramLink(int, int64, bool) error         { return nil }
func (r *taskBranchUserRepoStub) GetByIDSimple(int) (*models.User, error)           { return nil, nil }
func (r *taskBranchUserRepoStub) GetTelegramSettings(context.Context, int64) (int64, bool, error) {
	return 0, false, nil
}
func (r *taskBranchUserRepoStub) GetByChatID(context.Context, int64) (*models.User, error) {
	return nil, nil
}

func TestTaskHandler_GetByID_OperationsForbiddenForForeignBranch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	branchB := int64(2)
	svc := &taskBranchServiceStub{task: &models.Task{ID: 99, CreatorID: 11, AssigneeID: 11, BranchID: &branchB}}
	users := &taskBranchUserRepoStub{users: map[int]*models.User{10: {ID: 10, BranchID: ptrInt(1)}}}
	h := NewTaskHandler(svc, nil, users)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/tasks/99", nil)
	c.Params = gin.Params{{Key: "id", Value: "99"}}
	c.Set("user_id", 10)
	c.Set("role_id", authz.RoleOperations)

	h.GetByID(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestTaskHandler_ChangeStatus_OperationsAllowedForOwnBranch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	branch := int64(3)
	svc := &taskBranchServiceStub{
		task: &models.Task{ID: 55, CreatorID: 10, AssigneeID: 11, BranchID: &branch, Status: models.StatusNew},
	}
	users := &taskBranchUserRepoStub{users: map[int]*models.User{10: {ID: 10, BranchID: ptrInt(3)}}}
	h := NewTaskHandler(svc, nil, users)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks/55/status", strings.NewReader(`{"to":"in_progress"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "55"}}
	c.Set("user_id", 10)
	c.Set("role_id", authz.RoleOperations)

	h.ChangeStatus(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if svc.updateStatusCall != 1 {
		t.Fatalf("expected UpdateStatus to be called once, got %d", svc.updateStatusCall)
	}
}

func ptrInt(v int) *int { return &v }
