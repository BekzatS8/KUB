package services

import (
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type OrganizationService interface {
	Get() (*models.Organization, error)
	Update(req *models.UpdateOrganizationRequest) (*models.Organization, error)
}

type organizationService struct{ repo repositories.OrganizationRepository }

func NewOrganizationService(repo repositories.OrganizationRepository) OrganizationService {
	return &organizationService{repo: repo}
}

func (s *organizationService) Get() (*models.Organization, error) {
	return s.repo.Get()
}

func (s *organizationService) Update(req *models.UpdateOrganizationRequest) (*models.Organization, error) {
	return s.repo.Update(req)
}
