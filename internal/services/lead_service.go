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
	UserRepo  repositories.UserRepository
}

func (s *LeadService) branchScopeForRole(userID, roleID int) (*int, error) {
	switch roleID {
	case authz.RoleSales, authz.RoleOperations, authz.RoleControl:
		if s.UserRepo == nil {
			return nil, ErrForbidden
		}
		u, err := s.UserRepo.GetByID(userID)
		if err != nil || u == nil || u.BranchID == nil {
			return nil, ErrForbidden
		}
		return u.BranchID, nil
	case authz.RoleManagement, authz.RoleSystemAdmin:
		return nil, nil
	default:
		return nil, ErrForbidden
	}
}

func sameLeadBranch(required *int, lead *models.Leads) bool {
	if required == nil {
		return true
	}
	if lead == nil || lead.BranchID == nil {
		return false
	}
	return *required == *lead.BranchID
}

func NewLeadService(leadRepo *repositories.LeadRepository, dealRepo *repositories.DealRepository, clientRepo *repositories.ClientRepository, userRepo ...repositories.UserRepository) *LeadService {
	var clientSvc *ClientService
	if clientRepo != nil {
		clientSvc = NewClientService(clientRepo)
	}
	svc := &LeadService{Repo: leadRepo, DealRepo: dealRepo, ClientSvc: clientSvc}
	if len(userRepo) > 0 {
		svc.UserRepo = userRepo[0]
	}
	return svc
}

// Create: возвращаем ID созданного лида
func (s *LeadService) Create(lead *models.Leads, userID, roleID int) (int64, error) {
	if authz.IsReadOnly(roleID) {
		return 0, ErrReadOnly
	}
	if roleID == authz.RoleSales {
		lead.OwnerID = userID
	}
	if s.UserRepo != nil {
		if u, err := s.UserRepo.GetByID(userID); err == nil && u != nil {
			lead.BranchID = u.BranchID
		}
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
	branchScope, scopeErr := s.branchScopeForRole(userID, roleID)
	if scopeErr != nil {
		return scopeErr
	}
	if !sameLeadBranch(branchScope, current) {
		return ErrForbidden
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
	return s.Repo.ListAllWithFilterAndArchiveScope(limit, offset, repositories.LeadListFilter{}, repositories.ArchiveScopeActiveOnly)
}

func (s *LeadService) ListAllWithArchiveScope(limit, offset int, scope repositories.ArchiveScope) ([]*models.Leads, error) {
	return s.Repo.ListAllWithFilterAndArchiveScope(limit, offset, repositories.LeadListFilter{}, scope)
}

func (s *LeadService) ListMy(ownerID, limit, offset int) ([]*models.Leads, error) {
	return s.Repo.ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset, repositories.LeadListFilter{}, repositories.ArchiveScopeActiveOnly)
}

func (s *LeadService) ListMyWithArchiveScope(ownerID, limit, offset int, scope repositories.ArchiveScope) ([]*models.Leads, error) {
	return s.Repo.ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset, repositories.LeadListFilter{}, scope)
}

func (s *LeadService) ListForRole(userID, roleID, limit, offset int, scope repositories.ArchiveScope, filter repositories.LeadListFilter) ([]*models.Leads, error) {
	if roleID == authz.RoleSales {
		return nil, ErrForbidden
	}
	if branchScope, err := s.branchScopeForRole(userID, roleID); err == nil && branchScope != nil {
		filter.BranchID = branchScope
	}
	return s.Repo.ListAllWithFilterAndArchiveScope(limit, offset, filter, scope)
}

func (s *LeadService) ListMyWithFilterAndArchiveScope(ownerID, limit, offset int, scope repositories.ArchiveScope, filter repositories.LeadListFilter) ([]*models.Leads, error) {
	return s.Repo.ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset, filter, scope)
}

func (s *LeadService) ListForRoleWithTotal(userID, roleID, limit, offset int, scope repositories.ArchiveScope, filter repositories.LeadListFilter) ([]*models.Leads, int, error) {
	items, err := s.ListForRole(userID, roleID, limit, offset, scope, filter)
	if err != nil {
		return nil, 0, err
	}
	if branchScope, err := s.branchScopeForRole(userID, roleID); err == nil && branchScope != nil {
		filter.BranchID = branchScope
	}
	total, err := s.Repo.CountAllWithFilterAndArchiveScope(filter, scope)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *LeadService) ListMyWithFilterAndArchiveScopeAndTotal(ownerID, limit, offset int, scope repositories.ArchiveScope, filter repositories.LeadListFilter) ([]*models.Leads, int, error) {
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

func (s *LeadService) GetByID(id int, userID, roleID int) (*models.Leads, error) {
	lead, err := s.Repo.GetByID(id)
	if err != nil || lead == nil {
		return lead, err
	}
	if roleID == authz.RoleSales && lead.OwnerID != userID {
		return nil, ErrForbidden
	}
	branchScope, scopeErr := s.branchScopeForRole(userID, roleID)
	if scopeErr != nil {
		return nil, scopeErr
	}
	if !sameLeadBranch(branchScope, lead) {
		return nil, ErrForbidden
	}
	return lead, nil
}

func (s *LeadService) GetByIDWithArchiveScope(id int, userID, roleID int, scope repositories.ArchiveScope) (*models.Leads, error) {
	lead, err := s.Repo.GetByIDWithArchiveScope(id, scope)
	if err != nil || lead == nil {
		return lead, err
	}
	if roleID == authz.RoleSales && lead.OwnerID != userID {
		return nil, ErrForbidden
	}
	branchScope, scopeErr := s.branchScopeForRole(userID, roleID)
	if scopeErr != nil {
		return nil, scopeErr
	}
	if !sameLeadBranch(branchScope, lead) {
		return nil, ErrForbidden
	}
	return lead, nil
}

func (s *LeadService) Delete(id int, userID, roleID int) error {
	if !authz.CanHardDeleteBusinessEntity(roleID) {
		return ErrForbidden
	}
	lead, err := s.Repo.GetByIDWithArchiveScope(id, repositories.ArchiveScopeAll)
	if err != nil {
		return err
	}
	if lead == nil {
		return errors.New("lead not found")
	}
	branchScope, scopeErr := s.branchScopeForRole(userID, roleID)
	if scopeErr != nil {
		return scopeErr
	}
	if !sameLeadBranch(branchScope, lead) {
		return ErrForbidden
	}
	return s.Repo.Delete(id)
}

// ConvertLeadToDeal оставляем как у тебя, только напомню,
// что он требует lead.Status == "confirmed"
func (s *LeadService) ConvertLeadToDeal(leadID int, amount float64, currency string, ownerID, userID, roleID int, clientID int, clientType string) (*models.Deals, error) {
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
	normalizedClientType, err := normalizeRequiredDealClientType(clientType)
	if err != nil {
		return nil, err
	}
	lead, err := s.Repo.GetByID(leadID)
	if err != nil || lead == nil {
		return nil, errors.New("lead not found")
	}
	if roleID == authz.RoleSales && lead.OwnerID != userID {
		return nil, ErrForbidden
	}
	branchScope, scopeErr := s.branchScopeForRole(userID, roleID)
	if scopeErr != nil {
		return nil, scopeErr
	}
	if !sameLeadBranch(branchScope, lead) {
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
	if strings.ToLower(strings.TrimSpace(client.ClientType)) != normalizedClientType {
		return nil, ErrClientTypeMismatch
	}
	deal := &models.Deals{
		LeadID:     leadID,
		ClientID:   clientID,
		ClientType: normalizedClientType,
		OwnerID:    ownerID,
		Amount:     amount,
		Currency:   currency,
		Status:     "new",
		CreatedAt:  time.Now(),
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
	clientType, err := normalizeRequiredDealClientType(clientData.ClientType)
	if err != nil {
		return nil, err
	}
	clientData.ClientType = clientType
	if clientData.OwnerID == 0 {
		clientData.OwnerID = ownerID
	}
	if s.ClientSvc == nil {
		return nil, errors.New("client repository not configured")
	}

	client, err := s.ClientSvc.GetOrCreateByBIN(clientData.BinIin, clientData, userID, roleID)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, ErrClientNotFound
	}
	return s.ConvertLeadToDeal(leadID, amount, currency, ownerID, userID, roleID, client.ID, clientData.ClientType)
}

func (s *LeadService) UpdateStatus(id int, to string, userID, roleID int) error {
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
	branchScope, scopeErr := s.branchScopeForRole(userID, roleID)
	if scopeErr != nil {
		return scopeErr
	}
	if !sameLeadBranch(branchScope, lead) {
		return ErrForbidden
	}

	if !canTransition(lead.Status, to, LeadTransitions) {
		return errors.New("invalid status transition")
	}

	return s.Repo.UpdateStatus(id, to)
}

func (s *LeadService) ArchiveLead(id, userID, roleID int, reason string) error {
	if !authz.CanArchiveBusinessEntity(roleID) {
		return ErrForbidden
	}
	lead, err := s.Repo.GetByIDWithArchiveScope(id, repositories.ArchiveScopeAll)
	if err != nil {
		return err
	}
	if lead == nil {
		return ErrLeadNotFound
	}
	if roleID == authz.RoleSales && lead.OwnerID != userID {
		return ErrForbidden
	}
	branchScope, scopeErr := s.branchScopeForRole(userID, roleID)
	if scopeErr != nil {
		return scopeErr
	}
	if !sameLeadBranch(branchScope, lead) {
		return ErrForbidden
	}
	if lead.IsArchived {
		return nil
	}
	return s.Repo.Archive(id, userID, reason)
}

func (s *LeadService) UnarchiveLead(id, userID, roleID int) error {
	if !authz.CanArchiveBusinessEntity(roleID) {
		return ErrForbidden
	}
	lead, err := s.Repo.GetByIDWithArchiveScope(id, repositories.ArchiveScopeAll)
	if err != nil {
		return err
	}
	if lead == nil {
		return ErrLeadNotFound
	}
	if roleID == authz.RoleSales && lead.OwnerID != userID {
		return ErrForbidden
	}
	branchScope, scopeErr := s.branchScopeForRole(userID, roleID)
	if scopeErr != nil {
		return scopeErr
	}
	if !sameLeadBranch(branchScope, lead) {
		return ErrForbidden
	}
	if !lead.IsArchived {
		return ErrNotArchived
	}
	return s.Repo.Unarchive(id)
}

func (s *LeadService) AssignOwner(id, assigneeID, userID, roleID int) error {
	if roleID != authz.RoleManagement {
		return ErrForbidden
	}
	return s.Repo.UpdateOwner(id, assigneeID)
}
