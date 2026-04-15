package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
)

type authSelectCompanyAuthStub struct {
	lastUserID         int
	lastRoleID         int
	lastActiveCompany  *int
	generateTokenError error
}

func (s *authSelectCompanyAuthStub) VerifyPassword(hash, password string) bool { return true }
func (s *authSelectCompanyAuthStub) HashPassword(password string) (string, error) {
	return "", nil
}
func (s *authSelectCompanyAuthStub) GenerateAccessToken(userID, roleID int, activeCompanyID *int) (string, time.Time, error) {
	s.lastUserID = userID
	s.lastRoleID = roleID
	s.lastActiveCompany = activeCompanyID
	if s.generateTokenError != nil {
		return "", time.Time{}, s.generateTokenError
	}
	return "new-token", time.Now().Add(time.Hour), nil
}
func (s *authSelectCompanyAuthStub) GenerateRefreshToken() (string, time.Time, error) {
	return "", time.Time{}, nil
}

type authSelectCompanyCompanyStub struct {
	setErr        error
	setCalledWith int
}

func (s *authSelectCompanyCompanyStub) ListCompanies() ([]models.Company, error) { return nil, nil }
func (s *authSelectCompanyCompanyStub) GetCompany(id int) (*models.Company, error) {
	return nil, nil
}
func (s *authSelectCompanyCompanyStub) ListUserCompanies(userID int) ([]models.UserCompany, error) {
	return nil, nil
}
func (s *authSelectCompanyCompanyStub) ReplaceUserCompanies(userID int, companyIDs []int, primaryCompanyID *int) error {
	return nil
}
func (s *authSelectCompanyCompanyStub) HasUserAccess(userID, companyID int) (bool, error) {
	return true, nil
}
func (s *authSelectCompanyCompanyStub) GetPrimaryCompanyID(userID int) (*int, error) { return nil, nil }
func (s *authSelectCompanyCompanyStub) SetUserActiveCompany(userID int, companyID int) error {
	s.setCalledWith = companyID
	return s.setErr
}
func (s *authSelectCompanyCompanyStub) GetUserActiveCompanyID(userID int) (*int, error) {
	return nil, nil
}

func TestAuthHandler_SelectCompany_ReturnsAccessTokenWithActiveCompany(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authStub := &authSelectCompanyAuthStub{}
	companyStub := &authSelectCompanyCompanyStub{}
	h := NewAuthHandler(nil, authStub, nil, companyStub)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/select-company", strings.NewReader(`{"company_id": 17}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", 77)
	c.Set("role_id", 40)

	h.SelectCompany(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if companyStub.setCalledWith != 17 {
		t.Fatalf("expected SetUserActiveCompany called with 17, got %d", companyStub.setCalledWith)
	}
	if authStub.lastActiveCompany == nil || *authStub.lastActiveCompany != 17 {
		t.Fatalf("expected token issued for active company 17, got %#v", authStub.lastActiveCompany)
	}
	if !strings.Contains(w.Body.String(), "new-token") {
		t.Fatalf("expected response to contain new token, got %s", w.Body.String())
	}
}

func TestAuthHandler_SelectCompany_ForbiddenWithoutAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authStub := &authSelectCompanyAuthStub{}
	companyStub := &authSelectCompanyCompanyStub{setErr: errors.New("denied")}
	h := NewAuthHandler(nil, authStub, nil, companyStub)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/select-company", strings.NewReader(`{"company_id": 99}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", 77)
	c.Set("role_id", 40)

	h.SelectCompany(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}
