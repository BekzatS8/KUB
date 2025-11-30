package services

import (
	"errors"
	"time"

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

func (s *LeadService) Create(lead *models.Leads) error {
	if lead.Status == "" {
		lead.Status = "new"
	}
	if lead.CreatedAt.IsZero() {
		lead.CreatedAt = time.Now()
	}
	return s.Repo.Create(lead)
}

func (s *LeadService) Update(lead *models.Leads) error {
	return s.Repo.Update(lead)
}

func (s *LeadService) ListPaginated(limit, offset int) ([]*models.Leads, error) {
	return s.Repo.ListPaginated(limit, offset)
}

func (s *LeadService) ListMy(ownerID, limit, offset int) ([]*models.Leads, error) {
	return s.Repo.ListByOwner(ownerID, limit, offset)
}

func (s *LeadService) GetByID(id int) (*models.Leads, error) {
	return s.Repo.GetByID(id)
}

func (s *LeadService) Delete(id int) error {
	return s.Repo.Delete(id)
}

// ConvertLeadToDeal: добавили owner сделки (= owner лида)
func (s *LeadService) ConvertLeadToDeal(leadID int, amount, currency string, ownerID int, clientData *models.Client) (*models.Deals, error) {
	lead, err := s.Repo.GetByID(leadID)
	if err != nil || lead == nil {
		return nil, errors.New("lead not found")
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
func (s *LeadService) UpdateStatus(id int, to string) error {
	lead, err := s.Repo.GetByID(id)
	if err != nil || lead == nil {
		return err
	}
	if !canTransition(lead.Status, to, LeadTransitions) {
		return errors.New("invalid status transition")
	}
	return s.Repo.UpdateStatus(id, to)
}

func (s *LeadService) AssignOwner(id, assigneeID int) error {
	// простая бизнес-логика: просто смена owner_id
	return s.Repo.UpdateOwner(id, assigneeID)
}
