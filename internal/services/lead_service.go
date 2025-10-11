package services

import (
	"errors"
	"time"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type LeadService struct {
	Repo     *repositories.LeadRepository
	DealRepo *repositories.DealRepository
}

func NewLeadService(leadRepo *repositories.LeadRepository, dealRepo *repositories.DealRepository) *LeadService {
	return &LeadService{
		Repo:     leadRepo,
		DealRepo: dealRepo,
	}
}

func (s *LeadService) Create(lead *models.Leads) error {
	if s.Repo == nil {
		return errors.New("LeadRepository is not initialized")
	}
	if lead.Status == "" {
		lead.Status = "new"
	}
	return s.Repo.Create(lead)
}
func (s *LeadService) Update(lead *models.Leads) error {
	return s.Repo.Update(lead)
}

func (s *LeadService) ListPaginated(limit, offset int) ([]*models.Leads, error) {
	return s.Repo.ListPaginated(limit, offset)
}

func (s *LeadService) GetByID(id int) (*models.Leads, error) {
	return s.Repo.GetByID(id)
}

func (s *LeadService) Delete(id int) error {
	return s.Repo.Delete(id)
}

func (s *LeadService) ConvertLeadToDeal(leadID int, amount, currency string) (*models.Deals, error) {
	// Получаем лид
	lead, err := s.Repo.GetByID(leadID)
	if err != nil {
		return nil, errors.New("lead not found")
	}

	if lead.Status != "confirmed" {
		return nil, errors.New("lead is not in a convertible status")
	}

	// Проверяем, не существует ли уже сделка для этого лида
	existingDeal, err := s.DealRepo.GetByLeadID(leadID)
	if err != nil {
		return nil, err
	}
	if existingDeal != nil {
		return nil, errors.New("deal already exists for this lead")
	}

	// Создаем новую сделку
	deal := &models.Deals{
		LeadID:    lead.ID, // Теперь это int, а не string
		Amount:    amount,
		Currency:  currency,
		Status:    "new",
		CreatedAt: time.Now(),
	}

	// Сохраняем сделку и получаем её ID
	dealID, err := s.DealRepo.Create(deal)
	if err != nil {
		return nil, err
	}
	deal.ID = int(dealID)

	// Обновляем статус лида
	lead.Status = "converted"
	if err := s.Repo.Update(lead); err != nil {
		// Если не удалось обновить статус лида, можно попробовать откатить создание сделки
		_ = s.DealRepo.Delete(deal.ID)
		return nil, err
	}

	return deal, nil
}
