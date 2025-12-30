package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/models"
	"turcompany/internal/services"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type fakeUserService struct {
	user          *models.User
	storedRefresh string
	refreshExp    time.Time
}

func (f *fakeUserService) CreateUser(user *models.User) error { return nil }
func (f *fakeUserService) CreateUserWithPassword(user *models.User, plainPassword string) error {
	return nil
}
func (f *fakeUserService) GetUserByID(id int) (*models.User, error) { return nil, nil }
func (f *fakeUserService) UpdateUser(user *models.User) error       { return nil }
func (f *fakeUserService) DeleteUser(id int) error                  { return nil }
func (f *fakeUserService) ListUsers(limit, offset int) ([]*models.User, error) {
	return nil, nil
}
func (f *fakeUserService) GetUserByEmail(email string) (*models.User, error) {
	if f.user != nil && f.user.Email == email {
		return f.user, nil
	}
	return nil, nil
}
func (f *fakeUserService) GetUserCount() (int, error) { return 0, nil }
func (f *fakeUserService) GetUserCountByRole(roleID int) (int, error) {
	return 0, nil
}
func (f *fakeUserService) UpdateRefresh(userID int, token string, expiresAt time.Time) error {
	f.storedRefresh = token
	f.refreshExp = expiresAt
	return nil
}
func (f *fakeUserService) GetByRefreshToken(token string) (*models.User, error) { return nil, nil }
func (f *fakeUserService) RotateRefresh(oldToken, newToken string, newExpiresAt time.Time) (*models.User, error) {
	return nil, nil
}
func (f *fakeUserService) VerifyUser(userID int) error { return nil }

type fakePasswordResetService struct{}

func (fakePasswordResetService) RequestReset(email string) error               { return nil }
func (fakePasswordResetService) ResetPassword(token, newPassword string) error { return nil }

func TestAuthHandler_Login_Success(t *testing.T) {
	fixedNow := time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC)
	authSvc := services.NewAuthService([]byte("test-access-secret-32-bytes-long!!!"), []byte("test-refresh"), time.Minute, 5*time.Minute, func() time.Time {
		return fixedNow
	})

	hashed, err := authSvc.HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	user := &models.User{ID: 1, Email: "user@example.com", PasswordHash: hashed, RoleID: 2, IsVerified: true}
	userSvc := &fakeUserService{user: user}
	handler := NewAuthHandler(userSvc, authSvc, fakePasswordResetService{})

	r := gin.Default()
	r.POST("/auth/login", handler.Login)

	body, _ := json.Marshal(models.LoginRequest{Email: "user@example.com", Password: "secret123"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Tokens map[string]string `json:"tokens"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Tokens["access_token"] == "" || resp.Tokens["refresh_token"] == "" {
		t.Fatalf("expected access and refresh tokens in response, got %+v", resp.Tokens)
	}
}

func TestAuthHandler_Login_InvalidPassword(t *testing.T) {
	fixedNow := time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC)
	authSvc := services.NewAuthService([]byte("test-access-secret-32-bytes-long!!!"), []byte("test-refresh"), time.Minute, 5*time.Minute, func() time.Time {
		return fixedNow
	})

	hashed, err := authSvc.HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	user := &models.User{ID: 1, Email: "user@example.com", PasswordHash: hashed, RoleID: 2, IsVerified: true}
	userSvc := &fakeUserService{user: user}
	handler := NewAuthHandler(userSvc, authSvc, fakePasswordResetService{})

	r := gin.Default()
	r.POST("/auth/login", handler.Login)

	body, _ := json.Marshal(models.LoginRequest{Email: "user@example.com", Password: "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	var apiErr APIError
	if err := json.Unmarshal(w.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if apiErr.ErrorCode != UnauthorizedCode {
		t.Errorf("error_code = %s, want %s", apiErr.ErrorCode, UnauthorizedCode)
	}
}

func TestGinModeIsTest(t *testing.T) {
	if gin.Mode() != gin.TestMode {
		t.Fatalf("gin mode = %s, want %s", gin.Mode(), gin.TestMode)
	}
}
