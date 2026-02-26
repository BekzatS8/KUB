package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type fakeClientService struct {
	patchFn func(id int, updates map[string]any, userID, roleID int) (*models.Client, error)
}

func (f *fakeClientService) Create(c *models.Client, userID, roleID int) (int64, error) {
	return 0, nil
}
func (f *fakeClientService) Update(c *models.Client, userID, roleID int) error { return nil }
func (f *fakeClientService) Patch(id int, updates map[string]any, userID, roleID int) (*models.Client, error) {
	if f.patchFn != nil {
		return f.patchFn(id, updates, userID, roleID)
	}
	return &models.Client{ID: id}, nil
}
func (f *fakeClientService) GetByID(id int, userID, roleID int) (*models.Client, error) {
	return &models.Client{ID: id}, nil
}
func (f *fakeClientService) ListForRole(userID, roleID, limit, offset int, clientType string) ([]*models.Client, error) {
	return nil, nil
}
func (f *fakeClientService) ListMine(userID, limit, offset int, clientType string) ([]*models.Client, error) {
	return nil, nil
}
func (f *fakeClientService) GetMissingYellow(ctx context.Context, clientID, userID, roleID int) ([]string, error) {
	return nil, nil
}

func setupPatchRouter(svc clientService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := &ClientHandler{Service: svc}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 11)
		c.Set("role_id", authz.RoleManagement)
		c.Next()
	})
	r.PATCH("/clients/:id", h.Patch)
	return r
}

func TestClientPatchSingleField(t *testing.T) {
	r := setupPatchRouter(&fakeClientService{patchFn: func(id int, updates map[string]any, userID, roleID int) (*models.Client, error) {
		if updates["email"] != "new@example.com" {
			t.Fatalf("email not passed")
		}
		return &models.Client{ID: id, Email: "new@example.com"}, nil
	}})
	req := httptest.NewRequest(http.MethodPatch, "/clients/7", strings.NewReader(`{"email":"new@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestClientPatchInvalidEmail(t *testing.T) {
	r := setupPatchRouter(&fakeClientService{patchFn: func(id int, updates map[string]any, userID, roleID int) (*models.Client, error) {
		return nil, services.ErrInvalidEmail
	}})
	req := httptest.NewRequest(http.MethodPatch, "/clients/7", strings.NewReader(`{"email":"{{clientEmail}}"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var er APIError
	_ = json.Unmarshal(w.Body.Bytes(), &er)
	if er.ErrorCode != InvalidEmailCode {
		t.Fatalf("expected INVALID_EMAIL, got %s", er.ErrorCode)
	}
}

func TestClientPatchInvalidDate(t *testing.T) {
	r := setupPatchRouter(&fakeClientService{})
	req := httptest.NewRequest(http.MethodPatch, "/clients/7", strings.NewReader(`{"birth_date":"31/01/2024"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var er APIError
	_ = json.Unmarshal(w.Body.Bytes(), &er)
	if er.ErrorCode != InvalidDateFormatCode {
		t.Fatalf("expected INVALID_DATE_FORMAT, got %s", er.ErrorCode)
	}
}

func TestClientPatchEmailConflict(t *testing.T) {
	r := setupPatchRouter(&fakeClientService{patchFn: func(id int, updates map[string]any, userID, roleID int) (*models.Client, error) {
		return nil, services.ErrEmailAlreadyUsed
	}})
	req := httptest.NewRequest(http.MethodPatch, "/clients/7", strings.NewReader(`{"email":"dup@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}
