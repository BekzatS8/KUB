package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type LeadService struct {
	Repo      *repositories.LeadRepository
	DealRepo  *repositories.DealRepository
	ClientSvc *ClientService
}

func NewLeadService(leadRepo *repositories.LeadRepository, dealRepo *repositories.DealRepository, clientRepo *repositories.ClientRepository) *LeadService {
	var clientSvc *ClientService
	if clientRepo != nil {
		clientSvc = NewClientService(clientRepo)
	}
	return &LeadService{Repo: leadRepo, DealRepo: dealRepo, ClientSvc: clientSvc}
}

// Create: возвращаем ID созданного лида
func (s *LeadService) Create(lead *models.Leads, userID, roleID int) (int64, error) {
	if authz.CanManageSystem(roleID) {
		return 0, ErrForbidden
	}
	if authz.IsReadOnly(roleID) {
		return 0, ErrReadOnly
	}
	if roleID == authz.RoleSales {
		lead.OwnerID = userID
	}
	if lead.OwnerID == 0 {
		lead.OwnerID = userID
	}
	if roleID == authz.RoleSales && lead.OwnerID != userID {
		return 0, ErrForbidden
	}
	if lead.Status == "" {
		lead.Status = "new"
	}
	// created_at нам вернёт репозиторий через RETURNING
	return s.Repo.Create(lead)
}

func (s *LeadService) Update(lead *models.Leads, userID, roleID int) error {
	if authz.CanManageSystem(roleID) {
		return ErrForbidden
	}
	if authz.IsReadOnly(roleID) {
		return ErrReadOnly
	}

	current, err := s.Repo.GetByID(lead.ID)
	if err != nil {
		return err
	}
	if current == nil {
		return errors.New("lead not found")
	}

	if roleID == authz.RoleSales && current.OwnerID != userID {
		return ErrForbidden
	}

	// owner запрещаем менять всем кроме management
	// owner
	if roleID != authz.RoleManagement {
		lead.OwnerID = current.OwnerID
	} else {
		// ✅ management: если owner не прислали — не затирать на 0
		if lead.OwnerID == 0 {
			lead.OwnerID = current.OwnerID
		}
	}

	// ✅ статус запрещаем менять через обычный Update
	if lead.Status == "" {
		lead.Status = current.Status
	} else if lead.Status != current.Status {
		return errors.New("status must be updated via /leads/:id/status")
	}

	// (опционально) если title/description пустые — оставляем старые
	if lead.Title == "" {
		lead.Title = current.Title
	}
	if lead.Description == "" {
		lead.Description = current.Description
	}

	return s.Repo.Update(lead)
}

func (s *LeadService) ListAll(limit, offset int) ([]*models.Leads, error) {
	return s.Repo.ListAll(limit, offset)
}

func (s *LeadService) ListMy(ownerID, limit, offset int) ([]*models.Leads, error) {
	return s.Repo.ListByOwner(ownerID, limit, offset)
}

func (s *LeadService) ListForRole(userID, roleID, limit, offset int) ([]*models.Leads, error) {
	if authz.CanManageSystem(roleID) {
		return nil, ErrForbidden
	}
	if roleID == authz.RoleSales {
		return nil, ErrForbidden
	}
	return s.ListAll(limit, offset)
}

func (s *LeadService) GetByID(id int, userID, roleID int) (*models.Leads, error) {
	if authz.CanManageSystem(roleID) {
		return nil, ErrForbidden
	}
	lead, err := s.Repo.GetByID(id)
	if err != nil || lead == nil {
		return lead, err
	}
	if roleID == authz.RoleSales && lead.OwnerID != userID {
		return nil, ErrForbidden
	}
	return lead, nil
}

func (s *LeadService) Delete(id int, userID, roleID int) error {
	if authz.CanManageSystem(roleID) {
		return ErrForbidden
	}
	if authz.IsReadOnly(roleID) {
		return ErrReadOnly
	}
	if roleID == authz.RoleOperations {
		return ErrForbidden
	}
	lead, err := s.Repo.GetByID(id)
	if err != nil {
		return err
	}
	if lead == nil {
		return errors.New("lead not found")
	}
	if roleID == authz.RoleSales && lead.OwnerID != userID {
		return ErrForbidden
	}
	return s.Repo.Delete(id)
}

// ConvertLeadToDeal оставляем как у тебя, только напомню,
// что он требует lead.Status == "confirmed"
func (s *LeadService) ConvertLeadToDeal(leadID int, amount float64, currency string, ownerID, userID, roleID int, clientID int) (*models.Deals, error) {
	if authz.CanManageSystem(roleID) {
		return nil, ErrForbidden
	}
	if authz.IsReadOnly(roleID) {
		return nil, ErrReadOnly
	}
	if amount <= 0 {
		return nil, ErrAmountInvalid
	}
	if strings.TrimSpace(currency) == "" {
		return nil, errors.New("currency is required")
	}
	if clientID <= 0 {
		return nil, ErrClientIDRequired
	}
	lead, err := s.Repo.GetByID(leadID)
	if err != nil || lead == nil {
		return nil, errors.New("lead not found")
	}
	if roleID == authz.RoleSales && lead.OwnerID != userID {
		return nil, ErrForbidden
	}
	if s.ClientSvc == nil {
		return nil, errors.New("client repository not configured")
	}
	client, err := s.ClientSvc.GetByID(clientID, userID, roleID)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, ErrClientNotFound
	}
	deal := &models.Deals{
		LeadID:    leadID,
		OwnerID:   ownerID,
		Amount:    amount,
		Currency:  currency,
		Status:    "new",
		CreatedAt: time.Now(),
	}
	converted, err := s.Repo.ConvertToDeal(context.Background(), leadID, deal, client)
	if err != nil {
		if errors.Is(err, repositories.ErrClientNotFound) {
			return nil, ErrClientNotFound
		}
		if errors.Is(err, repositories.ErrDealAlreadyExists) {
			return converted, ErrDealAlreadyExists
		}
		return nil, err
	}
	return converted, nil
}

func (s *LeadService) ConvertLeadToDealWithClientData(leadID int, amount float64, currency string, ownerID, userID, roleID int, clientData *models.Client) (*models.Deals, error) {
	if authz.CanManageSystem(roleID) {
		return nil, ErrForbidden
	}
	if authz.IsReadOnly(roleID) {
		return nil, ErrReadOnly
	}
	if amount <= 0 {
		return nil, ErrAmountInvalid
	}
	if strings.TrimSpace(currency) == "" {
		return nil, errors.New("currency is required")
	}
	if clientData == nil {
		return nil, errors.New("client data is required")
	}
	if clientData.OwnerID == 0 {
		clientData.OwnerID = ownerID
	}
	if s.ClientSvc == nil {
		return nil, errors.New("client repository not configured")
	}

	client, err := s.ClientSvc.GetOrCreateByBIN(clientData.BinIin, clientData)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, ErrClientNotFound
	}
	return s.ConvertLeadToDeal(leadID, amount, currency, ownerID, userID, roleID, client.ID)
}

func (s *LeadService) UpdateStatus(id int, to string, userID, roleID int) error {
	if authz.CanManageSystem(roleID) {
		return ErrForbidden
	}
	if authz.IsReadOnly(roleID) {
		return ErrReadOnly
	}

	lead, err := s.Repo.GetByID(id)
	if err != nil {
		return err
	}
	if lead == nil {
		return errors.New("lead not found")
	}

	if roleID == authz.RoleSales && lead.OwnerID != userID {
		return ErrForbidden
	}

	if !canTransition(lead.Status, to, LeadTransitions) {
		return errors.New("invalid status transition")
	}

	return s.Repo.UpdateStatus(id, to)
}

func (s *LeadService) AssignOwner(id, assigneeID, userID, roleID int) error {
	if roleID != authz.RoleManagement {
		return ErrForbidden
	}
	return s.Repo.UpdateOwner(id, assigneeID)
}
