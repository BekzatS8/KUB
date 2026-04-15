package services

import (
	"fmt"
	"sort"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type CompanyService interface {
	ListCompanies() ([]models.Company, error)
	GetCompany(id int) (*models.Company, error)
	ListUserCompanies(userID int) ([]models.UserCompany, error)
	ReplaceUserCompanies(userID int, companyIDs []int, primaryCompanyID *int) error
	HasUserAccess(userID, companyID int) (bool, error)
	GetPrimaryCompanyID(userID int) (*int, error)
	SetUserActiveCompany(userID int, companyID int) error
	GetUserActiveCompanyID(userID int) (*int, error)
}

type companyService struct {
	companyRepo     repositories.CompanyRepository
	userCompanyRepo repositories.UserCompanyRepository
	userActiveRepo  repositories.UserActiveCompanyRepository
}

func NewCompanyService(companyRepo repositories.CompanyRepository, userCompanyRepo repositories.UserCompanyRepository, userActiveRepo repositories.UserActiveCompanyRepository) CompanyService {
	return &companyService{companyRepo: companyRepo, userCompanyRepo: userCompanyRepo, userActiveRepo: userActiveRepo}
}

func (s *companyService) ListCompanies() ([]models.Company, error) {
	return s.companyRepo.List()
}

func (s *companyService) GetCompany(id int) (*models.Company, error) {
	return s.companyRepo.GetByID(id)
}

func (s *companyService) ListUserCompanies(userID int) ([]models.UserCompany, error) {
	return s.userCompanyRepo.ListByUserID(userID)
}

func (s *companyService) ReplaceUserCompanies(userID int, companyIDs []int, primaryCompanyID *int) error {
	companyIDs = normalizeCompanyIDs(companyIDs)
	if len(companyIDs) == 0 {
		return s.userCompanyRepo.ReplaceUserCompanies(userID, companyIDs, nil)
	}
	count, err := s.companyRepo.CountByIDs(companyIDs)
	if err != nil {
		return err
	}
	if count != len(companyIDs) {
		return fmt.Errorf("one or more company_ids do not exist")
	}
	return s.userCompanyRepo.ReplaceUserCompanies(userID, companyIDs, primaryCompanyID)
}

func (s *companyService) HasUserAccess(userID, companyID int) (bool, error) {
	return s.userCompanyRepo.HasAccess(userID, companyID)
}

func (s *companyService) GetPrimaryCompanyID(userID int) (*int, error) {
	return s.userCompanyRepo.GetPrimaryCompanyID(userID)
}

func (s *companyService) SetUserActiveCompany(userID int, companyID int) error {
	ok, err := s.HasUserAccess(userID, companyID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("company access denied")
	}
	return s.userActiveRepo.SetActiveCompanyID(userID, &companyID)
}

func (s *companyService) GetUserActiveCompanyID(userID int) (*int, error) {
	active, err := s.userActiveRepo.GetActiveCompanyID(userID)
	if err != nil {
		return nil, err
	}
	if active != nil {
		ok, accessErr := s.HasUserAccess(userID, *active)
		if accessErr != nil {
			return nil, accessErr
		}
		if ok {
			return active, nil
		}
	}
	return s.GetPrimaryCompanyID(userID)
}

func normalizeCompanyIDs(input []int) []int {
	if len(input) <= 1 {
		return input
	}
	seen := make(map[int]struct{}, len(input))
	out := make([]int, 0, len(input))
	for _, v := range input {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}
