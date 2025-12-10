package services

import (
	"errors"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type DealService struct {
	Repo *repositories.DealRepository
}

func NewDealService(repo *repositories.DealRepository) *DealService {
	return &DealService{Repo: repo}
}

func (s *DealService) Create(deal *models.Deals, userID, roleID int) (int64, error) {
	if roleID == authz.RoleAdminStaff {
		return 0, ErrForbidden
	}
	if authz.IsReadOnly(roleID) {
		return 0, ErrReadOnly
	}
	if roleID == authz.RoleSales {
		deal.OwnerID = userID
	}
	if deal.OwnerID == 0 {
		deal.OwnerID = userID
	}
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return 0, ErrForbidden
	}
	if deal.ClientID == 0 {
		return 0, errors.New("client_id is required")
	}
	if deal.Status == "" {
		deal.Status = "new"
	}
	return s.Repo.Create(deal)
}

func (s *DealService) Update(deal *models.Deals, userID, roleID int) error {
	if roleID == authz.RoleAdminStaff {
		return ErrForbidden
	}
	if authz.IsReadOnly(roleID) {
		return ErrReadOnly
	}
	current, err := s.Repo.GetByID(deal.ID)
	if err != nil {
		return err
	}
	if current == nil {
		return errors.New("deal not found")
	}
	if roleID == authz.RoleSales && current.OwnerID != userID {
		return ErrForbidden
	}
	if roleID != authz.RoleManagement {
		deal.OwnerID = current.OwnerID
	}
	return s.Repo.Update(deal)
}

func (s *DealService) GetByID(id int, userID, roleID int) (*models.Deals, error) {
	if roleID == authz.RoleAdminStaff {
		return nil, ErrForbidden
	}
	deal, err := s.Repo.GetByID(id)
	if err != nil || deal == nil {
		return deal, err
	}
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return nil, ErrForbidden
	}
	return deal, nil
}

func (s *DealService) Delete(id int, userID, roleID int) error {
	if roleID == authz.RoleAdminStaff {
		return ErrForbidden
	}
	if authz.IsReadOnly(roleID) {
		return ErrReadOnly
	}
	if roleID == authz.RoleOperations {
		return ErrForbidden
	}
	deal, err := s.Repo.GetByID(id)
	if err != nil {
		return err
	}
	if deal == nil {
		return errors.New("deal not found")
	}
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return ErrForbidden
	}
	return s.Repo.Delete(id)
}

func (s *DealService) ListAll(limit, offset int) ([]*models.Deals, error) {
	return s.Repo.ListAll(limit, offset)
}

func (s *DealService) ListMy(ownerID, limit, offset int) ([]*models.Deals, error) {
	return s.Repo.ListByOwner(ownerID, limit, offset)
}

func (s *DealService) ListForRole(userID, roleID, limit, offset int) ([]*models.Deals, error) {
	if roleID == authz.RoleAdminStaff {
		return nil, ErrForbidden
	}
	if roleID == authz.RoleSales {
		return nil, ErrForbidden
	}
	return s.ListAll(limit, offset)
}

func (s *DealService) GetByLeadID(leadID int) (*models.Deals, error) {
	return s.Repo.GetByLeadID(leadID)
}

func (s *DealService) UpdateStatus(id int, to string, userID, roleID int) error {
	if roleID == authz.RoleAdminStaff {
		return ErrForbidden
	}
	if authz.IsReadOnly(roleID) {
		return ErrReadOnly
	}
	deal, err := s.Repo.GetByID(id)
	if err != nil || deal == nil {
		return err
	}
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return ErrForbidden
	}
	if !canTransition(deal.Status, to, DealTransitions) {
		return errors.New("invalid status transition")
	}
	return s.Repo.UpdateStatus(id, to)
}
