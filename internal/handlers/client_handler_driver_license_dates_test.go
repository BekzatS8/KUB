package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type driverDateStubClientService struct {
	created *models.Client
	updated *models.Client
	patched map[string]any
}

func (s *driverDateStubClientService) Create(c *models.Client, userID, roleID int) (int64, error) {
	s.created = c
	return 1, nil
}
func (s *driverDateStubClientService) Update(c *models.Client, userID, roleID int) error {
	s.updated = c
	return nil
}
func (s *driverDateStubClientService) Delete(int, int, int) error                { return nil }
func (s *driverDateStubClientService) ArchiveClient(int, int, int, string) error { return nil }
func (s *driverDateStubClientService) UnarchiveClient(int, int, int) error       { return nil }
func (s *driverDateStubClientService) Patch(id int, updates map[string]any, userID, roleID int) (*models.Client, error) {
	s.patched = updates
	return &models.Client{ID: id}, nil
}
func (s *driverDateStubClientService) GetByID(id int, userID, roleID int) (*models.Client, error) {
	return &models.Client{ID: id, OwnerID: userID}, nil
}
func (s *driverDateStubClientService) GetByIDWithArchiveScope(id int, userID, roleID int, scope repositories.ArchiveScope) (*models.Client, error) {
	return &models.Client{ID: id}, nil
}
func (s *driverDateStubClientService) ListForRole(userID, roleID, limit, offset int, filter repositories.ClientListFilter, scope repositories.ArchiveScope) ([]*models.Client, error) {
	return nil, nil
}
func (s *driverDateStubClientService) ListMineWithArchiveScope(userID, limit, offset int, filter repositories.ClientListFilter, scope repositories.ArchiveScope) ([]*models.Client, error) {
	return nil, nil
}
func (s *driverDateStubClientService) ListIndividualsForRole(userID, roleID, limit, offset int, filter repositories.ClientListFilter, scope repositories.ArchiveScope) ([]*models.Client, error) {
	return nil, nil
}
func (s *driverDateStubClientService) ListCompaniesForRole(userID, roleID, limit, offset int, filter repositories.ClientListFilter, scope repositories.ArchiveScope) ([]*models.Client, error) {
	return nil, nil
}
func (s *driverDateStubClientService) GetMissingYellow(context.Context, int, int, int) ([]string, error) {
	return nil, nil
}
func (s *driverDateStubClientService) GetProfile(context.Context, int, int, int) (*services.ClientProfilePayload, error) {
	return nil, nil
}

func newClientDatesCtx(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", 100)
	c.Set("role_id", 50)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	return c, w
}

func TestClientHandler_Create_ParsesDriverLicenseDates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &driverDateStubClientService{}
	h := &ClientHandler{Service: svc}
	body := `{"client_type":"individual","last_name":"Doe","first_name":"John","phone":"77001112233","country":"KZ","trip_purpose":"tour","birth_date":"2026-01-02","driver_license_issue_date":"2024-01-10","driver_license_expire_date":"2034-01-10"}`
	c, w := newClientDatesCtx(http.MethodPost, "/clients", body)

	h.Create(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
	if svc.created == nil || svc.created.DriverLicenseIssueDate == nil || svc.created.DriverLicenseExpireDate == nil {
		t.Fatalf("expected driver license dates propagated to created model: %#v", svc.created)
	}
}

func TestClientHandler_Update_ParsesDriverLicenseDates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &driverDateStubClientService{}
	h := &ClientHandler{Service: svc}
	body := `{"client_type":"individual","driver_license_issue_date":"2024-02-03","driver_license_expire_date":"2034-02-03"}`
	c, w := newClientDatesCtx(http.MethodPut, "/clients/1", body)

	h.Update(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if svc.updated == nil || svc.updated.DriverLicenseIssueDate == nil || svc.updated.DriverLicenseExpireDate == nil {
		t.Fatalf("expected driver license dates propagated to update model: %#v", svc.updated)
	}
}

func TestClientHandler_Patch_ParsesDriverLicenseDates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &driverDateStubClientService{}
	h := &ClientHandler{Service: svc}
	body := `{"driver_license_issue_date":"2024-03-04","driver_license_expire_date":"2034-03-04"}`
	c, w := newClientDatesCtx(http.MethodPatch, "/clients/1", body)

	h.Patch(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if svc.patched == nil {
		t.Fatal("expected patch updates map")
	}
	for _, key := range []string{"driver_license_issue_date", "driver_license_expire_date"} {
		v, ok := svc.patched[key]
		if !ok {
			t.Fatalf("expected %s in updates map: %#v", key, svc.patched)
		}
		if _, ok := v.(*time.Time); !ok {
			t.Fatalf("expected %s to be *time.Time, got %T", key, v)
		}
	}
}

func TestCreateClientRequest_DriverLicenseDatesJSON(t *testing.T) {
	raw := []byte(`{"driver_license_issue_date":"2024-01-01","driver_license_expire_date":"2034-01-01"}`)
	var req createClientRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.DriverLicenseIssueDate == "" || req.DriverLicenseExpireDate == "" {
		t.Fatalf("expected new create json fields mapped: %#v", req)
	}
}
