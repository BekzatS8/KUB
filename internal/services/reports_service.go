package services

import (
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type ReportService struct {
	LeadRepo *repositories.LeadRepository
	DealRepo *repositories.DealRepository
}

func NewReportService(leadRepo *repositories.LeadRepository, dealRepo *repositories.DealRepository) *ReportService {
	return &ReportService{
		LeadRepo: leadRepo,
		DealRepo: dealRepo,
	}
}

func (s *ReportService) GetSummary() (map[string]int, error) {
	totalLeads, err := s.LeadRepo.CountLeads()
	if err != nil {
		return nil, err
	}

	totalDeals, err := s.DealRepo.CountDeals()
	if err != nil {
		return nil, err
	}

	return map[string]int{
		"totalLeads": totalLeads,
		"totalDeals": totalDeals,
	}, nil
}

func (s *ReportService) FilterLeads(
	status string,
	ownerID int,
	sortBy, order string,
	limit, offset int,
) ([]models.Leads, error) {
	return s.LeadRepo.FilterLeads(status, ownerID, sortBy, order, limit, offset)
}

func (s *ReportService) FilterDeals(
	status, from, to, currency string,
	amountMin, amountMax float64,
	sortBy, order string,
	limit, offset int,
) ([]models.Deals, error) {
	return s.DealRepo.FilterDeals(status, from, to, currency, sortBy, order, amountMin, amountMax, limit, offset)
}
