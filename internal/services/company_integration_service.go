package services

import (
	"database/sql"
	"errors"
	"strings"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

var ErrIntegrationAccessDenied = errors.New("integration access denied")

type CompanyIntegrationService interface {
	List(companyID, requesterID, requesterRole int) ([]models.CompanyIntegration, error)
	Create(companyID, requesterID, requesterRole int, integration *models.CompanyIntegration) error
	Update(companyID int, integrationID int64, requesterID, requesterRole int, integration *models.CompanyIntegration) error
	Delete(companyID int, integrationID int64, requesterID, requesterRole int) error
}

type companyIntegrationService struct {
	repo          repositories.CompanyIntegrationRepository
	companyAccess CompanyService
}

func NewCompanyIntegrationService(repo repositories.CompanyIntegrationRepository, companyAccess CompanyService) CompanyIntegrationService {
	return &companyIntegrationService{repo: repo, companyAccess: companyAccess}
}

func (s *companyIntegrationService) canRead(roleID int) bool {
	return roleID == authz.RoleManagement || roleID == authz.RoleSystemAdmin
}

func (s *companyIntegrationService) canManage(roleID int) bool {
	return roleID == authz.RoleManagement || roleID == authz.RoleSystemAdmin
}

func (s *companyIntegrationService) ensureCompanyAccess(userID, companyID int) error {
	has, err := s.companyAccess.HasUserAccess(userID, companyID)
	if err != nil {
		return err
	}
	if !has {
		return ErrIntegrationAccessDenied
	}
	return nil
}

func (s *companyIntegrationService) List(companyID, requesterID, requesterRole int) ([]models.CompanyIntegration, error) {
	if !s.canRead(requesterRole) {
		return nil, ErrIntegrationAccessDenied
	}
	if err := s.ensureCompanyAccess(requesterID, companyID); err != nil {
		return nil, err
	}
	return s.repo.ListByCompanyID(companyID)
}

func (s *companyIntegrationService) Create(companyID, requesterID, requesterRole int, integration *models.CompanyIntegration) error {
	if !s.canManage(requesterRole) {
		return ErrIntegrationAccessDenied
	}
	if err := s.ensureCompanyAccess(requesterID, companyID); err != nil {
		return err
	}
	integration.CompanyID = companyID
	integration.IntegrationType = strings.TrimSpace(strings.ToLower(integration.IntegrationType))
	if err := repositories.ValidateIntegrationType(integration.IntegrationType); err != nil {
		return err
	}
	if strings.TrimSpace(integration.Title) == "" {
		return errors.New("title is required")
	}
	return s.repo.Create(integration)
}

func (s *companyIntegrationService) Update(companyID int, integrationID int64, requesterID, requesterRole int, integration *models.CompanyIntegration) error {
	if !s.canManage(requesterRole) {
		return ErrIntegrationAccessDenied
	}
	if err := s.ensureCompanyAccess(requesterID, companyID); err != nil {
		return err
	}
	integration.ID = integrationID
	integration.CompanyID = companyID
	integration.IntegrationType = strings.TrimSpace(strings.ToLower(integration.IntegrationType))
	if err := repositories.ValidateIntegrationType(integration.IntegrationType); err != nil {
		return err
	}
	if strings.TrimSpace(integration.Title) == "" {
		return errors.New("title is required")
	}
	return s.repo.Update(integration)
}

func (s *companyIntegrationService) Delete(companyID int, integrationID int64, requesterID, requesterRole int) error {
	if !s.canManage(requesterRole) {
		return ErrIntegrationAccessDenied
	}
	if err := s.ensureCompanyAccess(requesterID, companyID); err != nil {
		return err
	}
	err := s.repo.Delete(companyID, integrationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return err
	}
	return nil
}
