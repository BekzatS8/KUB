package services

import (
	"errors"
	"strings"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type DealService struct {
	Repo       *repositories.DealRepository
	ClientRepo *repositories.ClientRepository
	LeadRepo   *repositories.LeadRepository
	UserRepo   repositories.UserRepository
}

func NewDealService(repo *repositories.DealRepository, clientRepo ...*repositories.ClientRepository) *DealService {
	service := &DealService{Repo: repo}
	if len(clientRepo) > 0 {
		service.ClientRepo = clientRepo[0]
	}
	return service
}

func (s *DealService) SetScopeDeps(leadRepo *repositories.LeadRepository, userRepo repositories.UserRepository) {
	s.LeadRepo = leadRepo
	s.UserRepo = userRepo
}

func normalizeRequiredDealClientType(value string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return "", ErrClientTypeRequired
	}
	switch v {
	case models.ClientTypeIndividual, models.ClientTypeLegal:
		return v, nil
	default:
		return "", ErrInvalidClientType
	}
}

func (s *DealService) validateTypedClientRef(clientID int, clientType string) (string, error) {
	if clientID == 0 {
		return "", ErrClientIDRequired
	}
	requestedType, err := normalizeRequiredDealClientType(clientType)
	if err != nil {
		return "", err
	}
	if s.ClientRepo == nil {
		return "", ErrClientRepoNotConfigured
	}
	client, err := s.ClientRepo.GetByID(clientID)
	if err != nil {
		return "", err
	}
	if client == nil {
		return "", ErrClientNotFound
	}
	storedType, err := normalizeRequiredDealClientType(client.ClientType)
	if err != nil {
		return "", err
	}
	if storedType != requestedType {
		return "", ErrClientTypeMismatch
	}
	return requestedType, nil
}

func (s *DealService) Create(deal *models.Deals, userID, roleID int) (int64, error) {
	if authz.IsReadOnly(roleID) {
		return 0, ErrReadOnly
	}

	// Validate required fields first
	if deal.LeadID == 0 {
		return 0, ErrLeadIDRequired
	}
	if deal.Amount <= 0 {
		return 0, ErrAmountInvalid
	}
	clientType, err := s.validateTypedClientRef(deal.ClientID, deal.ClientType)
	if err != nil {
		return 0, err
	}
	deal.ClientType = clientType

	// Ownership rules
	if deal.OwnerID == 0 {
		deal.OwnerID = userID
	}
	if roleID == authz.RoleSales {
		// Sales can create deals only for themselves
		if deal.OwnerID != userID {
			return 0, ErrForbidden
		}
	}
	if s.LeadRepo != nil {
		if lead, err := s.LeadRepo.GetByID(deal.LeadID); err == nil && lead != nil {
			deal.BranchID = lead.BranchID
		}
	}
	if deal.BranchID == nil && s.UserRepo != nil {
		if u, err := s.UserRepo.GetByID(userID); err == nil && u != nil {
			deal.BranchID = u.BranchID
		}
	}

	if deal.Status == "" {
		deal.Status = "new"
	}

	id, err := s.Repo.Create(deal)
	if err != nil {
		if repositories.IsSQLState(err, repositories.SQLStateUniqueViolation) && repositories.ConstraintName(err) == "deals_lead_unique_idx" {
			existing, lookupErr := s.Repo.GetByLeadID(deal.LeadID)
			if lookupErr != nil {
				return 0, &DealAlreadyExistsError{LeadID: deal.LeadID}
			}
			existingID := 0
			if existing != nil {
				existingID = existing.ID
			}
			return 0, &DealAlreadyExistsError{LeadID: deal.LeadID, ExistingDealID: existingID}
		}
		if repositories.IsSQLState(err, repositories.SQLStateForeignKey) {
			pqErr, _ := repositories.AsPQError(err)
			fkMeta := strings.ToLower(string(pqErr.Constraint) + " " + pqErr.Message + " " + pqErr.Detail)
			switch {
			case strings.Contains(fkMeta, "lead"):
				return 0, ErrLeadNotFound
			case strings.Contains(fkMeta, "client"):
				return 0, ErrClientNotFound
			}
		}
		if repositories.IsSQLState(err, repositories.SQLStateCheckViolation) {
			return 0, ErrInvalidState
		}
		return 0, err
	}
	return id, nil
}

func (s *DealService) Update(deal *models.Deals, userID, roleID int) error {
	// 1) Базовые проверки ролей
	if authz.IsReadOnly(roleID) {
		return ErrReadOnly
	}

	// 2) Получаем текущую сделку
	current, err := s.Repo.GetByID(deal.ID)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrDealNotFound
	}

	// 3) Проверка доступа для sales
	if roleID == authz.RoleSales && current.OwnerID != userID {
		return ErrForbidden
	}

	// 4) Заполняем пропущенные поля из current

	if deal.LeadID == 0 {
		deal.LeadID = current.LeadID
	}
	if deal.LeadID == 0 {
		return ErrLeadIDRequired
	}

	if deal.ClientID == 0 {
		return ErrClientIDRequired
	}
	clientType, err := s.validateTypedClientRef(deal.ClientID, deal.ClientType)
	if err != nil {
		return err
	}
	deal.ClientType = clientType

	if deal.Amount == 0 {
		deal.Amount = current.Amount
	}
	if deal.Amount <= 0 {
		return ErrAmountInvalid
	}

	if deal.Currency == "" {
		deal.Currency = current.Currency
	}

	if deal.Status == "" {
		deal.Status = current.Status
	}
	if deal.Status == "" {
		deal.Status = "new"
	}

	// 5) Логика owner
	if roleID != authz.RoleManagement {
		// всем кроме management запрещаем менять владельца
		deal.OwnerID = current.OwnerID
	} else {
		// если management не передал owner_id — оставляем текущий
		if deal.OwnerID == 0 {
			deal.OwnerID = current.OwnerID
		}
	}

	// 6) Сохраняем изменения в БД
	err = s.Repo.Update(deal)
	if err != nil {
		if repositories.IsSQLState(err, repositories.SQLStateUniqueViolation) && repositories.ConstraintName(err) == "deals_lead_unique_idx" {
			return &DealAlreadyExistsError{LeadID: deal.LeadID}
		}
		if repositories.IsSQLState(err, repositories.SQLStateForeignKey) {
			pqErr, _ := repositories.AsPQError(err)
			fkMeta := strings.ToLower(string(pqErr.Constraint) + " " + pqErr.Message + " " + pqErr.Detail)
			switch {
			case strings.Contains(fkMeta, "lead"):
				return ErrLeadNotFound
			case strings.Contains(fkMeta, "client"):
				return ErrClientNotFound
			}
		}
		if repositories.IsSQLState(err, repositories.SQLStateCheckViolation) {
			return ErrInvalidState
		}
		return err
	}
	return nil
}

func (s *DealService) GetByID(id int, userID, roleID int) (*models.Deals, error) {
	deal, err := s.Repo.GetByID(id)
	if err != nil || deal == nil {
		return deal, err
	}
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return nil, ErrForbidden
	}
	if roleID == authz.RoleOperations && s.UserRepo != nil {
		if u, err := s.UserRepo.GetByID(userID); err == nil && u != nil && u.BranchID != nil && deal.BranchID != nil && *u.BranchID != *deal.BranchID {
			return nil, ErrForbidden
		}
	}
	return deal, nil
}

func (s *DealService) GetByIDWithArchiveScope(id int, userID, roleID int, scope repositories.ArchiveScope) (*models.Deals, error) {
	deal, err := s.Repo.GetByIDWithArchiveScope(id, scope)
	if err != nil || deal == nil {
		return deal, err
	}
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return nil, ErrForbidden
	}
	return deal, nil
}

func (s *DealService) Delete(id int, userID, roleID int) error {
	if !authz.CanHardDeleteBusinessEntity(roleID) {
		return ErrForbidden
	}
	deal, err := s.Repo.GetByIDWithArchiveScope(id, repositories.ArchiveScopeAll)
	if err != nil {
		return err
	}
	if deal == nil {
		return errors.New("deal not found")
	}
	return s.Repo.Delete(id)
}

func (s *DealService) ListAll(limit, offset int) ([]*models.Deals, error) {
	return s.Repo.ListAllWithFilterAndArchiveScope(limit, offset, repositories.DealListFilter{}, repositories.ArchiveScopeActiveOnly)
}

func (s *DealService) ListAllWithArchiveScope(limit, offset int, scope repositories.ArchiveScope) ([]*models.Deals, error) {
	return s.Repo.ListAllWithFilterAndArchiveScope(limit, offset, repositories.DealListFilter{}, scope)
}

func (s *DealService) ListMy(ownerID, limit, offset int) ([]*models.Deals, error) {
	return s.Repo.ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset, repositories.DealListFilter{}, repositories.ArchiveScopeActiveOnly)
}

func (s *DealService) ListMyWithArchiveScope(ownerID, limit, offset int, scope repositories.ArchiveScope) ([]*models.Deals, error) {
	return s.Repo.ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset, repositories.DealListFilter{}, scope)
}

func (s *DealService) ListForRole(userID, roleID, limit, offset int, scope repositories.ArchiveScope, filter repositories.DealListFilter) ([]*models.Deals, error) {
	if roleID == authz.RoleSales {
		return nil, ErrForbidden
	}
	if roleID == authz.RoleOperations && s.UserRepo != nil {
		if u, err := s.UserRepo.GetByID(userID); err == nil && u != nil && u.BranchID != nil {
			filter.BranchID = u.BranchID
		}
	}
	return s.Repo.ListAllWithFilterAndArchiveScope(limit, offset, filter, scope)
}

func (s *DealService) ListMyWithFilterAndArchiveScope(ownerID, limit, offset int, scope repositories.ArchiveScope, filter repositories.DealListFilter) ([]*models.Deals, error) {
	return s.Repo.ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset, filter, scope)
}

func (s *DealService) ListForRoleWithTotal(userID, roleID, limit, offset int, scope repositories.ArchiveScope, filter repositories.DealListFilter) ([]*models.Deals, int, error) {
	items, err := s.ListForRole(userID, roleID, limit, offset, scope, filter)
	if err != nil {
		return nil, 0, err
	}
	if roleID == authz.RoleOperations && s.UserRepo != nil {
		if u, err := s.UserRepo.GetByID(userID); err == nil && u != nil && u.BranchID != nil {
			filter.BranchID = u.BranchID
		}
	}
	total, err := s.Repo.CountAllWithFilterAndArchiveScope(filter, scope)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *DealService) ListMyWithFilterAndArchiveScopeAndTotal(ownerID, limit, offset int, scope repositories.ArchiveScope, filter repositories.DealListFilter) ([]*models.Deals, int, error) {
	items, err := s.Repo.ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset, filter, scope)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.Repo.CountByOwnerWithFilterAndArchiveScope(ownerID, filter, scope)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *DealService) GetByLeadID(leadID int) (*models.Deals, error) {
	return s.Repo.GetByLeadID(leadID)
}

func (s *DealService) UpdateStatus(id int, to string, userID, roleID int) error {
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

func (s *DealService) ArchiveDeal(id, userID, roleID int, reason string) error {
	if !authz.CanArchiveBusinessEntity(roleID) {
		return ErrForbidden
	}
	deal, err := s.Repo.GetByIDWithArchiveScope(id, repositories.ArchiveScopeAll)
	if err != nil {
		return err
	}
	if deal == nil {
		return ErrDealNotFound
	}
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return ErrForbidden
	}
	if deal.IsArchived {
		return nil
	}
	return s.Repo.Archive(id, userID, reason)
}

func (s *DealService) UnarchiveDeal(id, userID, roleID int) error {
	if !authz.CanArchiveBusinessEntity(roleID) {
		return ErrForbidden
	}
	deal, err := s.Repo.GetByIDWithArchiveScope(id, repositories.ArchiveScopeAll)
	if err != nil {
		return err
	}
	if deal == nil {
		return ErrDealNotFound
	}
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return ErrForbidden
	}
	if !deal.IsArchived {
		return ErrNotArchived
	}
	return s.Repo.Unarchive(id)
}
