package services

import (
	"errors"
	"testing"

	"turcompany/internal/models"
)

// ── stub repo ──────────────────────────────────────────────────────────────────

type stubOrgRepo struct {
	org    *models.Organization
	getErr error
	updErr error
}

func (r *stubOrgRepo) Get() (*models.Organization, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.org == nil {
		return &models.Organization{ID: 1, Name: ""}, nil
	}
	return r.org, nil
}

func (r *stubOrgRepo) Update(req *models.UpdateOrganizationRequest) (*models.Organization, error) {
	if r.updErr != nil {
		return nil, r.updErr
	}
	o := &models.Organization{ID: 1}
	if req.Name != nil {
		o.Name = *req.Name
	}
	if req.Phone != nil {
		o.Phone = *req.Phone
	}
	if req.WhatsApp != nil {
		o.WhatsApp = *req.WhatsApp
	}
	r.org = o
	return o, nil
}

// ── tests ──────────────────────────────────────────────────────────────────────

func TestOrganizationService_GetReturnsSingleton(t *testing.T) {
	repo := &stubOrgRepo{org: &models.Organization{ID: 1, Name: "KUB Travel"}}
	svc := NewOrganizationService(repo)

	org, err := svc.Get()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if org.ID != 1 {
		t.Errorf("want id=1, got %d", org.ID)
	}
	if org.Name != "KUB Travel" {
		t.Errorf("want name=KUB Travel, got %q", org.Name)
	}
}

func TestOrganizationService_GetPropagatesRepoError(t *testing.T) {
	repo := &stubOrgRepo{getErr: errors.New("db down")}
	svc := NewOrganizationService(repo)

	_, err := svc.Get()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestOrganizationService_UpdateChangesFields(t *testing.T) {
	repo := &stubOrgRepo{}
	svc := NewOrganizationService(repo)

	name := "Новая компания"
	phone := "+77001234567"
	wa := "77001234567"
	req := &models.UpdateOrganizationRequest{
		Name:     &name,
		Phone:    &phone,
		WhatsApp: &wa,
	}

	org, err := svc.Update(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if org.Name != name {
		t.Errorf("want name=%q, got %q", name, org.Name)
	}
	if org.Phone != phone {
		t.Errorf("want phone=%q, got %q", phone, org.Phone)
	}
	if org.WhatsApp != wa {
		t.Errorf("want whatsapp=%q, got %q", wa, org.WhatsApp)
	}
}

func TestOrganizationService_UpdatePropagatesRepoError(t *testing.T) {
	repo := &stubOrgRepo{updErr: errors.New("constraint violation")}
	svc := NewOrganizationService(repo)

	name := "x"
	_, err := svc.Update(&models.UpdateOrganizationRequest{Name: &name})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
