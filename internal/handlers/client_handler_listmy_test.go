package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"turcompany/internal/models"
	"turcompany/internal/services"
)

type stubClientListMyService struct {
	clients         []*models.Client
	err             error
	lastClientType  string
}

func (s *stubClientListMyService) Create(*models.Client, int, int) (int64, error) { return 0, errors.New("not implemented") }
func (s *stubClientListMyService) Update(*models.Client, int, int) error          { return errors.New("not implemented") }
func (s *stubClientListMyService) Delete(int, int, int) error                      { return errors.New("not implemented") }
func (s *stubClientListMyService) Patch(int, map[string]any, int, int) (*models.Client, error) {
	return nil, errors.New("not implemented")
}
func (s *stubClientListMyService) GetByID(int, int, int) (*models.Client, error) { return nil, errors.New("not implemented") }
func (s *stubClientListMyService) ListForRole(int, int, int, int, string) ([]*models.Client, error) {
	return nil, errors.New("not implemented")
}
func (s *stubClientListMyService) ListMine(_ int, _ int, _ int, clientType string) ([]*models.Client, error) {
	s.lastClientType = clientType
	return s.clients, s.err
}
func (s *stubClientListMyService) ListIndividualsForRole(int, int, int, int, string) ([]*models.Client, error) {
	return nil, errors.New("not implemented")
}
func (s *stubClientListMyService) ListCompaniesForRole(int, int, int, int, string) ([]*models.Client, error) {
	return nil, errors.New("not implemented")
}
func (s *stubClientListMyService) GetMissingYellow(_ context.Context, _, _, _ int) ([]string, error) {
	return nil, errors.New("not implemented")
}
func (s *stubClientListMyService) GetProfile(_ context.Context, _, _, _ int) (*services.ClientProfilePayload, error) {
	return nil, errors.New("not implemented")
}

func TestClientHandler_ListMy_SuccessScenarios(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		clients []*models.Client
	}{
		{
			name: "individual clients",
			clients: []*models.Client{{
				ID:         1,
				ClientType: models.ClientTypeIndividual,
				FirstName:  "A",
				LastName:   "B",
			}},
		},
		{
			name: "legal clients",
			clients: []*models.Client{{
				ID:         2,
				ClientType: models.ClientTypeLegal,
				Name:       "LLP Test",
				LegalProfile: &models.ClientLegalProfile{
					CompanyName: "LLP Test",
					BIN:         "123",
				},
			}},
		},
		{
			name:    "empty list",
			clients: []*models.Client{},
		},
		{
			name: "mixed legacy and profile",
			clients: []*models.Client{{
				ID:         3,
				ClientType: models.ClientTypeIndividual,
				Name:       "Legacy Person",
				FirstName:  "Legacy",
				LastName:   "Person",
			}, {
				ID:         4,
				ClientType: models.ClientTypeLegal,
				Name:       "Profile LLC",
				LegalProfile: &models.ClientLegalProfile{
					CompanyName: "Profile LLC",
					BIN:         "987",
				},
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &ClientHandler{Service: &stubClientListMyService{clients: tc.clients}}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/clients/my", nil)
			c.Set("user_id", 101)
			c.Set("role_id", 10)

			h.ListMy(c)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			var got []map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("invalid json: %v", err)
			}
			if len(got) != len(tc.clients) {
				t.Fatalf("expected %d clients, got %d", len(tc.clients), len(got))
			}
		})
	}
}

func TestClientHandler_ListMy_ForwardsOptionalClientType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &stubClientListMyService{clients: []*models.Client{}}
	h := &ClientHandler{Service: svc}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/clients/my?client_type=legal", nil)
	c.Set("user_id", 101)
	c.Set("role_id", 10)

	h.ListMy(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if svc.lastClientType != "legal" {
		t.Fatalf("expected client_type legal passed to service, got %q", svc.lastClientType)
	}
}
