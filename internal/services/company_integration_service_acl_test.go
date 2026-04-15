package services

import (
	"errors"
	"testing"

	"turcompany/internal/authz"
	"turcompany/internal/models"
)

type companyIntegrationRepoStub struct{}

func (s *companyIntegrationRepoStub) ListByCompanyID(companyID int) ([]models.CompanyIntegration, error) {
	return []models.CompanyIntegration{}, nil
}
func (s *companyIntegrationRepoStub) Create(integration *models.CompanyIntegration) error { return nil }
func (s *companyIntegrationRepoStub) Update(integration *models.CompanyIntegration) error { return nil }
func (s *companyIntegrationRepoStub) Delete(companyID int, integrationID int64) error     { return nil }
func (s *companyIntegrationRepoStub) GetByID(companyID int, integrationID int64) (*models.CompanyIntegration, error) {
	return nil, nil
}

type companyAccessStub struct{ has bool }

func (s *companyAccessStub) ListCompanies() ([]models.Company, error) { return nil, nil }
func (s *companyAccessStub) GetCompany(id int) (*models.Company, error) {
	return nil, nil
}
func (s *companyAccessStub) ListUserCompanies(userID int) ([]models.UserCompany, error) {
	return nil, nil
}
func (s *companyAccessStub) ReplaceUserCompanies(userID int, companyIDs []int, primaryCompanyID *int) error {
	return nil
}
func (s *companyAccessStub) HasUserAccess(userID, companyID int) (bool, error)    { return s.has, nil }
func (s *companyAccessStub) GetPrimaryCompanyID(userID int) (*int, error)         { return nil, nil }
func (s *companyAccessStub) SetUserActiveCompany(userID int, companyID int) error { return nil }
func (s *companyAccessStub) GetUserActiveCompanyID(userID int) (*int, error)      { return nil, nil }

func TestCompanyIntegrationService_List_DeniesNonLeadership(t *testing.T) {
	svc := NewCompanyIntegrationService(&companyIntegrationRepoStub{}, &companyAccessStub{has: true})
	_, err := svc.List(1, 10, authz.RoleSales)
	if !errors.Is(err, ErrIntegrationAccessDenied) {
		t.Fatalf("expected ErrIntegrationAccessDenied, got %v", err)
	}
}

func TestCompanyIntegrationService_List_RequiresMembership(t *testing.T) {
	svc := NewCompanyIntegrationService(&companyIntegrationRepoStub{}, &companyAccessStub{has: false})
	_, err := svc.List(1, 10, authz.RoleManagement)
	if !errors.Is(err, ErrIntegrationAccessDenied) {
		t.Fatalf("expected ErrIntegrationAccessDenied, got %v", err)
	}
}

func TestCompanyIntegrationService_List_AllowsLeadershipWithMembership(t *testing.T) {
	svc := NewCompanyIntegrationService(&companyIntegrationRepoStub{}, &companyAccessStub{has: true})
	_, err := svc.List(1, 10, authz.RoleManagement)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
