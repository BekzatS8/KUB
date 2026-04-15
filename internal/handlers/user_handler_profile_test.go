package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
)

type userProfileUserServiceStub struct {
	user *models.User
}

func (s *userProfileUserServiceStub) CreateUser(user *models.User) error { return nil }
func (s *userProfileUserServiceStub) CreateUserWithPassword(user *models.User, plainPassword string) error {
	return nil
}
func (s *userProfileUserServiceStub) GetUserByID(id int) (*models.User, error) { return s.user, nil }
func (s *userProfileUserServiceStub) UpdateUser(user *models.User) error       { return nil }
func (s *userProfileUserServiceStub) DeleteUser(id int) error                  { return nil }
func (s *userProfileUserServiceStub) ListUsers(limit, offset int) ([]*models.User, error) {
	return nil, nil
}
func (s *userProfileUserServiceStub) GetUserByEmail(email string) (*models.User, error) {
	return nil, nil
}
func (s *userProfileUserServiceStub) GetUserCount() (int, error)                 { return 0, nil }
func (s *userProfileUserServiceStub) GetUserCountByRole(roleID int) (int, error) { return 0, nil }
func (s *userProfileUserServiceStub) UpdateRefresh(userID int, token string, expiresAt time.Time) error {
	return nil
}
func (s *userProfileUserServiceStub) GetByRefreshToken(token string) (*models.User, error) {
	return nil, nil
}
func (s *userProfileUserServiceStub) RotateRefresh(oldToken, newToken string, newExpiresAt time.Time) (*models.User, error) {
	return nil, nil
}
func (s *userProfileUserServiceStub) VerifyUser(userID int) error { return nil }

type userProfileCompanyServiceStub struct{}

func (s *userProfileCompanyServiceStub) ListCompanies() ([]models.Company, error) { return nil, nil }
func (s *userProfileCompanyServiceStub) GetCompany(id int) (*models.Company, error) {
	return nil, nil
}
func (s *userProfileCompanyServiceStub) ListUserCompanies(userID int) ([]models.UserCompany, error) {
	return []models.UserCompany{{CompanyID: 1, IsPrimary: true, Company: &models.Company{ID: 1, Name: "A"}}, {CompanyID: 2, Company: &models.Company{ID: 2, Name: "B"}}}, nil
}
func (s *userProfileCompanyServiceStub) ReplaceUserCompanies(userID int, companyIDs []int, primaryCompanyID *int) error {
	return nil
}
func (s *userProfileCompanyServiceStub) HasUserAccess(userID, companyID int) (bool, error) {
	return true, nil
}
func (s *userProfileCompanyServiceStub) GetPrimaryCompanyID(userID int) (*int, error) {
	v := 1
	return &v, nil
}
func (s *userProfileCompanyServiceStub) SetUserActiveCompany(userID int, companyID int) error {
	return nil
}
func (s *userProfileCompanyServiceStub) GetUserActiveCompanyID(userID int) (*int, error) {
	v := 2
	return &v, nil
}

func TestUserHandler_GetMyProfile_ContainsMultiCompanyBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewUserHandler(&userProfileUserServiceStub{user: &models.User{ID: 10, Email: "u@example.com", RoleID: 40, IsVerified: true}}, nil, &userProfileCompanyServiceStub{})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/users/me", nil)
	c.Set("user_id", 10)
	c.Set("role_id", 40)

	h.GetMyProfile(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, token := range []string{"\"companies\"", "\"primary_company_id\"", "\"active_company_id\""} {
		if !strings.Contains(body, token) {
			t.Fatalf("expected profile response to contain %s, body=%s", token, body)
		}
	}
}
