package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
)

type companyACLServiceStub struct {
	hasAccess bool
}

func (s *companyACLServiceStub) ListCompanies() ([]models.Company, error) { return nil, nil }
func (s *companyACLServiceStub) GetCompany(id int) (*models.Company, error) {
	return &models.Company{ID: id, Name: "C"}, nil
}
func (s *companyACLServiceStub) ListUserCompanies(userID int) ([]models.UserCompany, error) {
	return []models.UserCompany{
		{CompanyID: 11, Company: &models.Company{ID: 11, Name: "A"}},
		{CompanyID: 22, Company: &models.Company{ID: 22, Name: "B"}},
	}, nil
}
func (s *companyACLServiceStub) ReplaceUserCompanies(userID int, companyIDs []int, primaryCompanyID *int) error {
	return nil
}
func (s *companyACLServiceStub) HasUserAccess(userID, companyID int) (bool, error) {
	return s.hasAccess, nil
}
func (s *companyACLServiceStub) GetPrimaryCompanyID(userID int) (*int, error)         { return nil, nil }
func (s *companyACLServiceStub) SetUserActiveCompany(userID int, companyID int) error { return nil }
func (s *companyACLServiceStub) GetUserActiveCompanyID(userID int) (*int, error)      { return nil, nil }

func TestCompanyHandler_List_IsMembershipScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewCompanyHandler(&companyACLServiceStub{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/companies", nil)
	c.Set("user_id", 7)
	c.Set("role_id", 10)

	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !(contains(body, "\"id\":11") && contains(body, "\"id\":22")) {
		t.Fatalf("expected only membership-scoped companies, got %s", body)
	}
}

func TestCompanyHandler_GetByID_ReturnsNotFoundWithoutMembership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewCompanyHandler(&companyACLServiceStub{hasAccess: false})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/companies/55", nil)
	c.Params = gin.Params{{Key: "id", Value: "55"}}
	c.Set("user_id", 7)
	c.Set("role_id", 10)

	h.GetByID(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

func contains(s, needle string) bool { return strings.Contains(s, needle) }
