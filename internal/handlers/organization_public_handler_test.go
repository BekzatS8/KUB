package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
)

type stubOrgService struct {
	org *models.Organization
}

func (s *stubOrgService) Get() (*models.Organization, error) { return s.org, nil }
func (s *stubOrgService) Update(_ *models.UpdateOrganizationRequest) (*models.Organization, error) {
	return s.org, nil
}

func TestGetPublicContactsNoJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	orgSvc := &stubOrgService{org: &models.Organization{
		ID:        1,
		Name:      "Тур Компания",
		LegalName: "ТОО «Тур Компания»",
		BIN:       "987654321012",
		Phone:     "+77001234567",
		Email:     "info@turco.kz",
		Website:   "https://turco.kz",
		WhatsApp:  "+77001234567",
		Telegram:  "@turco",
		Instagram: "@turco_kz",
		TikTok:    "@turco",
		LogoURL:   "https://cdn.turco.kz/logo.png",
		Address:   "г. Алматы, ул. Примерная 1",
	}}
	h := NewOrganizationHandler(orgSvc)
	r := gin.New()
	// No authMiddleware — simulates the public registration order in routes.go
	r.GET("/api/v1/public/organization/contacts", h.GetPublicContacts)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/public/organization/contacts", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// White-listed fields must be present
	for _, field := range []string{"name", "phone", "email", "website", "whatsapp", "telegram", "instagram", "tiktok", "logo_url"} {
		if _, ok := body[field]; !ok {
			t.Errorf("expected field %q in response, missing", field)
		}
	}
	if body["name"] != "Тур Компания" {
		t.Errorf("expected name %q, got %v", "Тур Компания", body["name"])
	}

	// Private fields must NOT be present
	for _, field := range []string{"id", "bin", "legal_name", "address", "created_at", "updated_at"} {
		if _, ok := body[field]; ok {
			t.Errorf("private field %q must not appear in public contacts response", field)
		}
	}

	// CORS: open to any origin
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected Access-Control-Allow-Origin: *, got %q", got)
	}
}

