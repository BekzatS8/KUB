package services

import (
	"errors"
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
	if roleID == authz.RoleAdminStaff {
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
	if roleID == authz.RoleAdminStaff {
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
	if roleID != authz.RoleManagement {
		lead.OwnerID = current.OwnerID
	}
	// created_at не трогаем — его вообще не обновляем в репозитории
	return s.Repo.Update(lead)
}

func (s *LeadService) ListAll(limit, offset int) ([]*models.Leads, error) {
	return s.Repo.ListAll(limit, offset)
}

func (s *LeadService) ListMy(ownerID, limit, offset int) ([]*models.Leads, error) {
	return s.Repo.ListByOwner(ownerID, limit, offset)
}

func (s *LeadService) ListForRole(userID, roleID, limit, offset int) ([]*models.Leads, error) {
	if roleID == authz.RoleAdminStaff {
		return nil, ErrForbidden
	}
	if roleID == authz.RoleSales {
		return nil, ErrForbidden
	}
	return s.ListAll(limit, offset)
}

func (s *LeadService) GetByID(id int, userID, roleID int) (*models.Leads, error) {
	if roleID == authz.RoleAdminStaff {
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
	if roleID == authz.RoleAdminStaff {
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
func (s *LeadService) ConvertLeadToDeal(leadID int, amount, currency string, ownerID, userID, roleID int, clientData *models.Client) (*models.Deals, error) {
	if roleID == authz.RoleAdminStaff {
		return nil, ErrForbidden
	}
	if authz.IsReadOnly(roleID) {
		return nil, ErrReadOnly
	}
	lead, err := s.Repo.GetByID(leadID)
	if err != nil || lead == nil {
		return nil, errors.New("lead not found")
	}
	if roleID == authz.RoleSales && lead.OwnerID != userID {
		return nil, ErrForbidden
	}
	// допустимый статус для конвертации
	if lead.Status != "confirmed" {
		return nil, errors.New("lead is not in a convertible status")
	}

	// идемпотентность — не создаём вторую сделку
	existingDeal, err := s.DealRepo.GetByLeadID(leadID)
	if err != nil {
		return nil, err
	}
	if existingDeal != nil {
		return nil, errors.New("deal already exists for this lead")
	}
	if s.ClientSvc == nil {
		return nil, errors.New("client repository not configured")
	}

	if clientData == nil {
		return nil, errors.New("client data is required")
	}
	if clientData.OwnerID == 0 {
		clientData.OwnerID = ownerID
	}
	var client *models.Client
	client, err = s.ClientSvc.GetOrCreateByBIN(clientData.BinIin, clientData)
	if err != nil {
		return nil, err
	}
	deal := &models.Deals{
		LeadID:    lead.ID,
		ClientID:  client.ID,
		OwnerID:   ownerID,
		Amount:    amount,
		Currency:  currency,
		Status:    "new",
		CreatedAt: time.Now(),
	}

	dealID, err := s.DealRepo.Create(deal)
	if err != nil {
		return nil, err
	}
	deal.ID = int(dealID)

	lead.Status = "converted"
	if err := s.Repo.Update(lead); err != nil {
		_ = s.DealRepo.Delete(deal.ID) // best-effort rollback
		return nil, err
	}
	return deal, nil
}

func (s *LeadService) UpdateStatus(id int, to string, userID, roleID int) error {
	if roleID == authz.RoleAdminStaff {
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
