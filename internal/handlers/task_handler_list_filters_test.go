package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type stubTaskListService struct {
	lastFilter models.TaskFilter
	called     bool
	total      int
}

func (s *stubTaskListService) Create(context.Context, *models.Task) (*models.Task, error) {
	return nil, nil
}
func (s *stubTaskListService) GetByID(context.Context, int64) (*models.Task, error) { return nil, nil }
func (s *stubTaskListService) GetByIDWithArchiveScope(context.Context, int64, repositories.ArchiveScope) (*models.Task, error) {
	return nil, nil
}
func (s *stubTaskListService) GetAll(_ context.Context, filter models.TaskFilter) ([]models.Task, error) {
	s.called = true
	s.lastFilter = filter
	return []models.Task{}, nil
}
func (s *stubTaskListService) GetAllPaginated(_ context.Context, filter models.TaskFilter, _, _ int) ([]models.Task, int, error) {
	s.called = true
	s.lastFilter = filter
	return []models.Task{}, s.total, nil
}
func (s *stubTaskListService) Update(context.Context, int64, *models.Task) (*models.Task, error) {
	return nil, nil
}

func TestTaskHandler_GetAll_PaginatedEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &stubTaskListService{total: 31}
	h := NewTaskHandler(svc, nil, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/tasks?paginate=true&page=-2&size=500", nil)
	c.Set("user_id", 500)
	c.Set("role_id", authz.RoleManagement)

	h.GetAll(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "\"items\":") || !strings.Contains(body, "\"pagination\":") {
		t.Fatalf("expected paginated envelope, got %s", body)
	}
	if !strings.Contains(body, "\"page\":1") || !strings.Contains(body, "\"size\":100") {
		t.Fatalf("expected normalized page/size, got %s", body)
	}
}
func (s *stubTaskListService) Delete(context.Context, int64, int64, int) error { return nil }
func (s *stubTaskListService) ArchiveTask(context.Context, int64, int64, int, string) (*models.Task, error) {
	return nil, nil
}
func (s *stubTaskListService) UnarchiveTask(context.Context, int64, int64, int) (*models.Task, error) {
	return nil, nil
}
func (s *stubTaskListService) UpdateStatus(context.Context, int64, models.TaskStatus) (*models.Task, error) {
	return nil, nil
}
func (s *stubTaskListService) UpdateAssignee(context.Context, int64, int64) (*models.Task, error) {
	return nil, nil
}

func TestTaskHandler_GetAll_ForwardsExtendedFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &stubTaskListService{}
	h := NewTaskHandler(svc, nil, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/tasks?q=Smoke&status_group=active&assignee_id=123&creator_id=77&entity_id=13&entity_type=lead&sort_by=due_date&order=asc&archive=all", nil)
	c.Set("user_id", 500)
	c.Set("role_id", authz.RoleManagement)

	h.GetAll(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !svc.called {
		t.Fatal("expected service.GetAll to be called")
	}
	if svc.lastFilter.Query != "Smoke" || svc.lastFilter.StatusGroup != "active" || svc.lastFilter.SortBy != "due_date" || svc.lastFilter.Order != "asc" || svc.lastFilter.Archive != "all" {
		t.Fatalf("unexpected filter: %+v", svc.lastFilter)
	}
	if svc.lastFilter.AssigneeID == nil || *svc.lastFilter.AssigneeID != 123 {
		t.Fatalf("expected assignee_id=123, got %+v", svc.lastFilter.AssigneeID)
	}
}

func TestTaskHandler_GetAll_SalesForcedToOwnAssignee(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &stubTaskListService{}
	h := NewTaskHandler(svc, nil, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/tasks?assignee_id=123&status_group=active", nil)
	c.Set("user_id", 42)
	c.Set("role_id", authz.RoleSales)

	h.GetAll(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if svc.lastFilter.AssigneeID == nil || *svc.lastFilter.AssigneeID != 42 {
		t.Fatalf("expected assignee forced to 42, got %+v", svc.lastFilter.AssigneeID)
	}
}

func TestTaskHandler_GetAll_InvalidFilters(t *testing.T) {
	tests := []string{
		"/tasks?status=unknown",
		"/tasks?status_group=progress",
		"/tasks?sort_by=amount",
		"/tasks?order=up",
		"/tasks?assignee_id=bad",
		"/tasks?creator_id=bad",
		"/tasks?entity_id=bad",
	}
	for _, url := range tests {
		gin.SetMode(gin.TestMode)
		svc := &stubTaskListService{}
		h := NewTaskHandler(svc, nil, nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, url, nil)
		c.Set("user_id", 1)
		c.Set("role_id", authz.RoleManagement)

		h.GetAll(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("url=%s expected 400 got %d body=%s", url, w.Code, w.Body.String())
		}
	}
}
