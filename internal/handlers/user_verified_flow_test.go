package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type stubUserService struct {
	createdUser *models.User
	createErr   error
	byEmail     *models.User
}

func (s *stubUserService) CreateUser(*models.User) error { return nil }
func (s *stubUserService) CreateUserWithPassword(user *models.User, _ string) error {
	cp := *user
	s.createdUser = &cp
	user.ID = 101
	if user.IsVerified && user.VerifiedAt == nil {
		now := time.Now().UTC()
		user.VerifiedAt = &now
	}
	return s.createErr
}
func (s *stubUserService) GetUserByID(int) (*models.User, error)          { return nil, nil }
func (s *stubUserService) UpdateUser(*models.User) error                  { return nil }
func (s *stubUserService) DeleteUser(int) error                           { return nil }
func (s *stubUserService) ListUsers(int, int) ([]*models.User, error)     { return nil, nil }
func (s *stubUserService) GetUserByEmail(string) (*models.User, error)    { return s.byEmail, nil }
func (s *stubUserService) GetUserCount() (int, error)                     { return 0, nil }
func (s *stubUserService) GetUserCountByRole(int) (int, error)            { return 0, nil }
func (s *stubUserService) UpdateRefresh(int, string, time.Time) error     { return nil }
func (s *stubUserService) GetByRefreshToken(string) (*models.User, error) { return nil, nil }
func (s *stubUserService) RotateRefresh(string, string, time.Time) (*models.User, error) {
	return nil, nil
}
func (s *stubUserService) VerifyUser(int) error { return nil }

func TestCreateUser_DefaultIsVerifiedFalseWhenFieldMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &stubUserService{}
	h := NewUserHandler(svc, nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 1)
		c.Set("role_id", authz.RoleSystemAdmin)
		c.Next()
	})
	r.POST("/users", h.CreateUser)

	body := map[string]interface{}{
		"company_name": "Acme",
		"bin_iin":      "123456789012",
		"email":        "admin-created@example.com",
		"password":     "Passw0rd",
		"phone":        "+77001112233",
		"role_id":      authz.RoleSales,
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	if svc.createdUser == nil {
		t.Fatal("expected service CreateUserWithPassword to be called")
	}
	if svc.createdUser.IsVerified {
		t.Fatal("expected default is_verified=false for POST /users without field")
	}
}

func TestCreateUser_WithIsVerifiedTruePassesFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &stubUserService{}
	h := NewUserHandler(svc, nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 1)
		c.Set("role_id", authz.RoleSystemAdmin)
		c.Next()
	})
	r.POST("/users", h.CreateUser)

	body := `{"company_name":"Acme","bin_iin":"123456789012","email":"verified@example.com","password":"Passw0rd","phone":"+77001112233","role_id":10,"is_verified":true}`
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	if svc.createdUser == nil || !svc.createdUser.IsVerified {
		t.Fatal("expected is_verified=true to be passed into create flow")
	}
}

func TestRegister_IgnoresIsVerifiedAndKeepsPublicFlowUnverified(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &stubUserService{}
	h := NewUserHandler(svc, nil)

	r := gin.New()
	r.POST("/register", h.Register)

	body := `{"company_name":"Acme","bin_iin":"123456789012","email":"public@example.com","password":"Passw0rd","phone":"+77001112233","is_verified":true}`
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	if svc.createdUser == nil {
		t.Fatal("expected service CreateUserWithPassword to be called")
	}
	if svc.createdUser.IsVerified {
		t.Fatal("expected /register to always create unverified users")
	}
}

func TestLogin_VerifiedUserCanLoginImmediately(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authSvc := services.NewAuthService([]byte("01234567890123456789012345678901"), nil, 0, 0, nil)
	hash, err := authSvc.HashPassword("Passw0rd")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	now := time.Now().UTC()
	svc := &stubUserService{byEmail: &models.User{
		ID:           7,
		Email:        "verified@example.com",
		PasswordHash: hash,
		RoleID:       authz.RoleSales,
		IsVerified:   true,
		VerifiedAt:   &now,
	}}
	h := NewAuthHandler(svc, authSvc, nil)

	r := gin.New()
	r.POST("/auth/login", h.Login)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(`{"email":"verified@example.com","password":"Passw0rd"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}
