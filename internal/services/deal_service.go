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
}

func NewDealService(repo *repositories.DealRepository, clientRepo ...*repositories.ClientRepository) *DealService {
	service := &DealService{Repo: repo}
	if len(clientRepo) > 0 {
		service.ClientRepo = clientRepo[0]
	}
	return service
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
		return "", errors.New("invalid client_type: allowed values are individual, legal")
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
		return "", errors.New("client repository not configured")
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
	if authz.CanManageSystem(roleID) {
		return 0, ErrForbidden
	}
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

	if deal.Status == "" {
		deal.Status = "new"
	}

	return s.Repo.Create(deal)
}

func (s *DealService) Update(deal *models.Deals, userID, roleID int) error {
	// 1) Базовые проверки ролей
	if authz.CanManageSystem(roleID) {
		return ErrForbidden
	}
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
	return s.Repo.Update(deal)
}

func (s *DealService) GetByID(id int, userID, roleID int) (*models.Deals, error) {
	if authz.CanManageSystem(roleID) {
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
	if authz.CanManageSystem(roleID) {
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
	if authz.CanManageSystem(roleID) {
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
	if authz.CanManageSystem(roleID) {
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
