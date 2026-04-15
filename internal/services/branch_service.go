package services

import (
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type BranchService interface {
	CreateBranch(branch *models.Branch) error
	GetBranchByID(id int) (*models.Branch, error)
	ListBranches() ([]*models.Branch, error)
	UpdateBranch(branch *models.Branch) error
	DeleteBranch(id int) error
}

type branchService struct{ repo repositories.BranchRepository }

func NewBranchService(repo repositories.BranchRepository) BranchService {
	return &branchService{repo: repo}
}

func (s *branchService) CreateBranch(branch *models.Branch) error     { return s.repo.Create(branch) }
func (s *branchService) GetBranchByID(id int) (*models.Branch, error) { return s.repo.GetByID(id) }
func (s *branchService) ListBranches() ([]*models.Branch, error)      { return s.repo.List() }
func (s *branchService) UpdateBranch(branch *models.Branch) error     { return s.repo.Update(branch) }
func (s *branchService) DeleteBranch(id int) error                    { return s.repo.Delete(id) }
