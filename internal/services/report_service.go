package services

import (
	"context"
	"time"

	"turcompany/internal/authz"
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

func (s *ReportService) resolveOwnerFilter(userID, roleID int) (ownerID *int, err error) {
	switch roleID {
	case authz.RoleSales:
		return &userID, nil
	case authz.RoleOperations, authz.RoleManagement, authz.RoleControl:
		return nil, nil
	case authz.RoleSystemAdmin:
		return nil, ErrForbidden
	default:
		return nil, ErrForbidden
	}
}

type SalesFunnelItem struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

type SalesFunnelReport struct {
	From  time.Time         `json:"from"`
	To    time.Time         `json:"to"`
	Items []SalesFunnelItem `json:"items"`
}

func (s *ReportService) GetSalesFunnel(ctx context.Context, from, to time.Time, userID, roleID int) (*SalesFunnelReport, error) {
	ownerID, err := s.resolveOwnerFilter(userID, roleID)
	if err != nil {
		return nil, err
	}

	rows, err := s.DealRepo.GetDealsFunnelStats(ctx, from, to, ownerID)
	if err != nil {
		return nil, err
	}

	items := make([]SalesFunnelItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, SalesFunnelItem{Status: row.Status, Count: row.Count})
	}

	return &SalesFunnelReport{
		From:  from,
		To:    to,
		Items: items,
	}, nil
}

type LeadsSummaryItem struct {
	Status string `json:"status"`
	Source string `json:"source"`
	Count  int64  `json:"count"`
}

type LeadsSummaryReport struct {
	From  time.Time          `json:"from"`
	To    time.Time          `json:"to"`
	Items []LeadsSummaryItem `json:"items"`
}

func (s *ReportService) GetLeadsSummary(ctx context.Context, from, to time.Time, userID, roleID int) (*LeadsSummaryReport, error) {
	ownerID, err := s.resolveOwnerFilter(userID, roleID)
	if err != nil {
		return nil, err
	}

	rows, err := s.LeadRepo.GetLeadsSummaryStats(ctx, from, to, ownerID)
	if err != nil {
		return nil, err
	}

	items := make([]LeadsSummaryItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, LeadsSummaryItem{Status: row.Status, Source: row.Source, Count: row.Count})
	}

	return &LeadsSummaryReport{
		From:  from,
		To:    to,
		Items: items,
	}, nil
}

type RevenueItem struct {
	Period      string  `json:"period"`
	TotalAmount float64 `json:"total_amount"`
	Currency    string  `json:"currency"`
}

type TopClientItem struct {
	ClientID    int     `json:"client_id"`
	ClientType  string  `json:"client_type"`
	ClientName  string  `json:"client_name"`
	TotalAmount float64 `json:"total_amount"`
	Currency    string  `json:"currency"`
}

type RevenueReport struct {
	From       time.Time       `json:"from"`
	To         time.Time       `json:"to"`
	Period     string          `json:"period"`
	Items      []RevenueItem   `json:"items"`
	TopClients []TopClientItem `json:"top_clients"`
}

func (s *ReportService) GetRevenueStats(ctx context.Context, from, to time.Time, userID, roleID int, period string) (*RevenueReport, error) {
	ownerID, err := s.resolveOwnerFilter(userID, roleID)
	if err != nil {
		return nil, err
	}

	revenueRows, err := s.DealRepo.GetDealsRevenueStats(ctx, from, to, ownerID)
	if err != nil {
		return nil, err
	}

	items := make([]RevenueItem, 0, len(revenueRows))
	for _, row := range revenueRows {
		items = append(items, RevenueItem{Period: row.Period, TotalAmount: row.TotalAmount, Currency: row.Currency})
	}

	topRows, err := s.DealRepo.GetTopClientsByRevenue(ctx, from, to, ownerID, 10)
	if err != nil {
		return nil, err
	}

	topItems := make([]TopClientItem, 0, len(topRows))
	for _, row := range topRows {
		topItems = append(topItems, TopClientItem{
			ClientID:    row.ClientID,
			ClientType:  row.ClientType,
			ClientName:  row.ClientName,
			TotalAmount: row.TotalAmount,
			Currency:    row.Currency,
		})
	}

	if period == "" {
		period = "month"
	}

	return &RevenueReport{
		From:       from,
		To:         to,
		Period:     period,
		Items:      items,
		TopClients: topItems,
	}, nil
}

type DashboardKPI struct {
	Key          string  `json:"key"`
	Value        float64 `json:"value"`
	TrendPercent float64 `json:"trend_percent"`
}

type DashboardKPIReport struct {
	From  time.Time      `json:"from"`
	To    time.Time      `json:"to"`
	Items []DashboardKPI `json:"items"`
}

func (s *ReportService) GetDashboardKPI(ctx context.Context, from, to time.Time, userID, roleID int) (*DashboardKPIReport, error) {
	ownerID, err := s.resolveOwnerFilter(userID, roleID)
	if err != nil {
		return nil, err
	}

	funnelRows, err := s.DealRepo.GetDealsFunnelStats(ctx, from, to, ownerID)
	if err != nil {
		return nil, err
	}

	revenueRows, err := s.DealRepo.GetDealsRevenueStats(ctx, from, to, ownerID)
	if err != nil {
		return nil, err
	}

	topClients, err := s.DealRepo.GetTopClientsByRevenue(ctx, from, to, ownerID, 0)
	if err != nil {
		return nil, err
	}

	var wonCount int64
	var totalDeals int64
	var totalRevenue float64
	for _, row := range funnelRows {
		totalDeals += row.Count
		if row.Status == "won" {
			wonCount = row.Count
		}
	}

	for _, row := range revenueRows {
		totalRevenue += row.TotalAmount
	}

	conversionRate := 0.0
	if totalDeals > 0 {
		conversionRate = float64(wonCount) / float64(totalDeals) * 100
	}

	items := []DashboardKPI{
		{Key: "total_revenue", Value: totalRevenue, TrendPercent: 0},
		{Key: "new_clients_count", Value: float64(len(topClients)), TrendPercent: 0},
		{Key: "closed_deals_count", Value: float64(wonCount), TrendPercent: 0},
		{Key: "conversion_rate", Value: conversionRate, TrendPercent: 0},
	}

	return &DashboardKPIReport{
		From:  from,
		To:    to,
		Items: items,
	}, nil
}
