package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"turcompany/internal/models"
	"turcompany/internal/services"
)

type fakeClientProfileService struct {
	payload *services.ClientProfilePayload
	err     error
}

func (f *fakeClientProfileService) GetProfile(ctx context.Context, clientID, userID, roleID int) (*services.ClientProfilePayload, error) {
	return f.payload, f.err
}

func TestClientProfileHandlerWithoutPhoto(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewClientProfileHandler(&fakeClientProfileService{payload: &services.ClientProfilePayload{
		Client:        &models.Client{ID: 1, Name: "Ivanov"},
		MissingYellow: []string{"photo35x45", "email"},
		PhotoExists:   false,
	}})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 10)
		c.Set("role_id", 4)
		c.Next()
	})
	r.GET("/clients/:id/profile", h.GetProfile)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/clients/1/profile", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	files := resp["files"].(map[string]any)
	photo := files["photo35x45"].(map[string]any)
	if photo["exists"].(bool) {
		t.Fatalf("expected exists=false")
	}
	comp := resp["completeness"].(map[string]any)
	missing := comp["missing_yellow"].([]any)
	found := false
	for _, v := range missing {
		if v.(string) == "photo35x45" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected missing_yellow contain photo35x45")
	}
}

func TestClientProfileHandlerWithPhoto(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewClientProfileHandler(&fakeClientProfileService{payload: &services.ClientProfilePayload{
		Client:        &models.Client{ID: 1, Name: "Ivanov"},
		MissingYellow: []string{"email"},
		PhotoExists:   true,
	}})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 10)
		c.Set("role_id", 4)
		c.Next()
	})
	r.GET("/clients/:id/profile", h.GetProfile)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/clients/1/profile", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	files := resp["files"].(map[string]any)
	photo := files["photo35x45"].(map[string]any)
	if !photo["exists"].(bool) {
		t.Fatalf("expected exists=true")
	}
	comp := resp["completeness"].(map[string]any)
	missing := comp["missing_yellow"].([]any)
	for _, v := range missing {
		if v.(string) == "photo35x45" {
			t.Fatalf("did not expect photo35x45 in missing_yellow")
		}
	}
}
