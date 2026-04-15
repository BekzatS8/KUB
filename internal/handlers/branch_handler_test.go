package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/authz"
	"turcompany/internal/models"
)

type stubBranchService struct {
	list      []*models.Branch
	byID      *models.Branch
	created   *models.Branch
	updated   *models.Branch
	deletedID int
}

func (s *stubBranchService) CreateBranch(branch *models.Branch) error {
	cp := *branch
	s.created = &cp
	branch.ID = 77
	return nil
}
func (s *stubBranchService) GetBranchByID(id int) (*models.Branch, error) {
	if s.byID != nil {
		return s.byID, nil
	}
	return &models.Branch{ID: id, Name: "B", Code: "B", IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}
func (s *stubBranchService) ListBranches() ([]*models.Branch, error) { return s.list, nil }
func (s *stubBranchService) UpdateBranch(branch *models.Branch) error {
	cp := *branch
	s.updated = &cp
	return nil
}
func (s *stubBranchService) DeleteBranch(id int) error { s.deletedID = id; return nil }

func TestBranchCRUDPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	branchSvc := &stubBranchService{list: []*models.Branch{{ID: 1, Name: "Branch 1", Code: "BRANCH_1", IsActive: true}}}
	userSvc := &stubUserService{byID: &models.User{ID: 9, RoleID: authz.RoleSales, BranchID: intPtr(1)}}
	h := NewBranchHandler(branchSvc, userSvc)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", 9); c.Set("role_id", authz.RoleSales); c.Next() })
	r.GET("/branches", h.List)
	r.POST("/branches", h.Create)
	r.GET("/branches/:id", h.GetByID)

	wList := httptest.NewRecorder()
	r.ServeHTTP(wList, httptest.NewRequest(http.MethodGet, "/branches", nil))
	if wList.Code != http.StatusOK {
		t.Fatalf("list expected 200 got %d", wList.Code)
	}

	wCreate := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/branches", bytes.NewBufferString(`{"name":"x","code":"x"}`))
	createReq.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wCreate, createReq)
	if wCreate.Code != http.StatusForbidden {
		t.Fatalf("create expected 403 got %d", wCreate.Code)
	}

	wGetOwn := httptest.NewRecorder()
	r.ServeHTTP(wGetOwn, httptest.NewRequest(http.MethodGet, "/branches/1", nil))
	if wGetOwn.Code != http.StatusOK {
		t.Fatalf("get own expected 200 got %d", wGetOwn.Code)
	}

	wGetOther := httptest.NewRecorder()
	r.ServeHTTP(wGetOther, httptest.NewRequest(http.MethodGet, "/branches/2", nil))
	if wGetOther.Code != http.StatusForbidden {
		t.Fatalf("get foreign expected 403 got %d", wGetOther.Code)
	}
}

func intPtr(v int) *int { return &v }
