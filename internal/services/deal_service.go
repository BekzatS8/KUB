package services

import (
	"errors"
	"strings"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type DealService struct {
	Repo               *repositories.DealRepository
	ClientRepo         *repositories.ClientRepository
	LeadRepo           *repositories.LeadRepository
	UserRepo           repositories.UserRepository
	StageRepo          *repositories.FunnelStageRepository
	TransitionRuleRepo *repositories.FunnelTransitionRuleRepository
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

func (s *DealService) SetStageRepo(stageRepo *repositories.FunnelStageRepository) {
	s.StageRepo = stageRepo
}

func (s *DealService) SetTransitionRuleRepo(repo *repositories.FunnelTransitionRuleRepository) {
	s.TransitionRuleRepo = repo
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
	dataScope, scopeErr := resolveDealScope(userID, roleID, s.UserRepo)
	if scopeErr != nil {
		return 0, scopeErr
	}
	if !dealMatchesScope(dataScope, deal) {
		return 0, ErrForbidden
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
	dataScope, scopeErr := resolveDealScope(userID, roleID, s.UserRepo)
	if scopeErr != nil {
		return scopeErr
	}
	if !dealMatchesScope(dataScope, current) {
		return ErrForbidden
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
	if roleID != authz.RoleManagement && roleID != authz.RoleSystemAdmin {
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
	dataScope, scopeErr := resolveDealScope(userID, roleID, s.UserRepo)
	if scopeErr != nil {
		return nil, scopeErr
	}
	if !dealMatchesScope(dataScope, deal) {
		return nil, ErrForbidden
	}
	return deal, nil
}

func (s *DealService) GetByIDWithArchiveScope(id int, userID, roleID int, scope repositories.ArchiveScope) (*models.Deals, error) {
	deal, err := s.Repo.GetByIDWithArchiveScope(id, scope)
	if err != nil || deal == nil {
		return deal, err
	}
	dataScope, scopeErr := resolveDealScope(userID, roleID, s.UserRepo)
	if scopeErr != nil {
		return nil, scopeErr
	}
	if !dealMatchesScope(dataScope, deal) {
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
	dataScope, scopeErr := resolveDealScope(userID, roleID, s.UserRepo)
	if scopeErr != nil {
		return scopeErr
	}
	if !dealMatchesScope(dataScope, deal) {
		return ErrForbidden
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
	// sales resolves to ScopeKindBranch (свой отдел+филиал) via resolveDealScope —
	// no hard guard. Was previously blocked, forcing sales onto /deals/my
	// (ListAll = все сделки), leaking every department's deals.
	dataScope, err := resolveDealScope(userID, roleID, s.UserRepo)
	if err != nil {
		return nil, err
	}
	return listDealsForScope(s.Repo, dataScope, limit, offset, filter, scope)
}

func (s *DealService) ListMyWithFilterAndArchiveScope(ownerID int, limit, offset int, scope repositories.ArchiveScope, filter repositories.DealListFilter) ([]*models.Deals, error) {
	// "Мои" = сделки текущего владельца. Ранее ошибочно возвращало ВСЕ сделки.
	return s.Repo.ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset, filter, scope)
}

func (s *DealService) ListForRoleWithTotal(userID, roleID, limit, offset int, scope repositories.ArchiveScope, filter repositories.DealListFilter) ([]*models.Deals, int, error) {
	items, err := s.ListForRole(userID, roleID, limit, offset, scope, filter)
	if err != nil {
		return nil, 0, err
	}
	dataScope, err := resolveDealScope(userID, roleID, s.UserRepo)
	if err != nil {
		return nil, 0, err
	}
	total, err := countDealsForScope(s.Repo, dataScope, filter, scope)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *DealService) ListMyWithFilterAndArchiveScopeAndTotal(ownerID int, limit, offset int, scope repositories.ArchiveScope, filter repositories.DealListFilter) ([]*models.Deals, int, error) {
	// "Мои" = сделки текущего владельца. Ранее возвращало ВСЕ сделки (ListAll).
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
	dataScope, scopeErr := resolveDealScope(userID, roleID, s.UserRepo)
	if scopeErr != nil {
		return scopeErr
	}
	if !dealMatchesScope(dataScope, deal) {
		return ErrForbidden
	}
	if !canTransition(deal.Status, to, DealTransitions) {
		return errors.New("invalid status transition")
	}
	return s.Repo.UpdateStatus(id, to)
}

// MoveStage moves a deal to a different funnel stage (kanban drag&drop) and
// records the transition in deal_stage_history. The deal's status is kept in
// sync with the stage type: moving into a "won"/"lost" stage sets the matching
// status, while moving a previously won/lost/cancelled deal into a regular
// stage resets it to "in_progress".
func (s *DealService) MoveStage(dealID, stageID int, comment string, userID, roleID int) error {
	if authz.IsReadOnly(roleID) {
		return ErrReadOnly
	}
	if s.StageRepo == nil {
		return ErrInvalidState
	}
	deal, err := s.Repo.GetByID(dealID)
	if err != nil {
		return err
	}
	if deal == nil {
		return ErrDealNotFound
	}
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return ErrForbidden
	}
	dataScope, scopeErr := resolveDealScope(userID, roleID, s.UserRepo)
	if scopeErr != nil {
		return scopeErr
	}
	if !dealMatchesScope(dataScope, deal) {
		return ErrForbidden
	}

	stage, err := s.StageRepo.GetByID(stageID)
	if err != nil {
		return err
	}
	if stage == nil {
		return ErrNotFound
	}
	if deal.FunnelID != nil && *deal.FunnelID != stage.FunnelID {
		return ErrInvalidState
	}

	// validDealStatuses maps the standard deal status values recognised by the
	// deals.status CHECK constraint. Used to derive status from stage.Code when
	// the stage is of type "regular".
	validDealStatuses := map[string]bool{
		"new": true, "in_progress": true, "negotiation": true,
		"won": true, "lost": true, "cancelled": true,
	}

	newStatus := deal.Status
	switch stage.Type {
	case models.FunnelStageTypeWon:
		newStatus = "won"
	case models.FunnelStageTypeLost:
		newStatus = "lost"
	default: // regular
		if validDealStatuses[stage.Code] {
			// Stage code matches a standard status (e.g. "new", "in_progress",
			// "negotiation") — keep them in sync so the kanban column reflects
			// the actual deal status badge.
			newStatus = stage.Code
		} else {
			// Custom-coded stage: always advance to in_progress.
			// This covers reopening won/lost/cancelled deals AND advancing
			// a "new" deal when dragged into any active stage column.
			newStatus = "in_progress"
		}
	}

	if err := s.Repo.MoveStage(dealID, stageID, stage.FunnelID, newStatus); err != nil {
		return err
	}

	history := &models.DealStageHistory{
		DealID:      dealID,
		FromStageID: deal.StageID,
		ToStageID:   &stageID,
		ChangedBy:   &userID,
		Comment:     comment,
	}
	if err := s.StageRepo.InsertHistory(history); err != nil {
		return err
	}

	// Apply admin-configured automatic cross-funnel transition rules.
	// Pass depth=1 to guard against circular rule chains (max 10 hops).
	_ = s.applyTransitionRules(dealID, stage.FunnelID, stageID, userID, 1)
	return nil
}

// applyTransitionRules checks if any active transition rules fire for the given
// (funnelID, stageID) and, if so, moves the deal accordingly. The depth guard
// prevents infinite loops from misconfigured circular rules.
func (s *DealService) applyTransitionRules(dealID, funnelID, stageID, changedBy, depth int) error {
	if depth > 10 || s.TransitionRuleRepo == nil || s.StageRepo == nil {
		return nil
	}

	rules, err := s.TransitionRuleRepo.FindActiveByTrigger(funnelID, stageID)
	if err != nil || len(rules) == 0 {
		return err
	}

	// Reload deal to have an up-to-date snapshot.
	deal, err := s.Repo.GetByID(dealID)
	if err != nil || deal == nil {
		return err
	}

	// Apply the first matching active rule (rules are ordered by id).
	rule := rules[0]
	toStage, err := s.StageRepo.GetByID(rule.ToStageID)
	if err != nil || toStage == nil {
		return err
	}

	validDealStatuses := map[string]bool{
		"new": true, "in_progress": true, "negotiation": true,
		"won": true, "lost": true, "cancelled": true,
	}
	newStatus := deal.Status
	switch toStage.Type {
	case models.FunnelStageTypeWon:
		newStatus = "won"
	case models.FunnelStageTypeLost:
		newStatus = "lost"
	default:
		if validDealStatuses[toStage.Code] {
			newStatus = toStage.Code
		} else {
			newStatus = "in_progress"
		}
	}

	if err := s.Repo.MoveStageAndFunnel(dealID, rule.ToStageID, rule.ToFunnelID, newStatus); err != nil {
		return err
	}

	autoComment := "Автоматический переход: " + rule.Name
	fromStageID := &stageID
	toStageID := rule.ToStageID
	history := &models.DealStageHistory{
		DealID:      dealID,
		FromStageID: fromStageID,
		ToStageID:   &toStageID,
		ChangedBy:   &changedBy,
		Comment:     autoComment,
	}
	if err := s.StageRepo.InsertHistory(history); err != nil {
		return err
	}

	// Recursively check if the new stage also triggers a rule.
	return s.applyTransitionRules(dealID, rule.ToFunnelID, rule.ToStageID, changedBy, depth+1)
}

// GetHistory returns the stage-transition history for a deal, enforcing the
// same access scope as reading the deal itself.
func (s *DealService) GetHistory(dealID, userID, roleID int) ([]*models.DealStageHistory, error) {
	if s.StageRepo == nil {
		return nil, ErrInvalidState
	}
	deal, err := s.Repo.GetByID(dealID)
	if err != nil {
		return nil, err
	}
	if deal == nil {
		return nil, ErrDealNotFound
	}
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return nil, ErrForbidden
	}
	dataScope, scopeErr := resolveDealScope(userID, roleID, s.UserRepo)
	if scopeErr != nil {
		return nil, scopeErr
	}
	if !dealMatchesScope(dataScope, deal) {
		return nil, ErrForbidden
	}
	return s.StageRepo.ListHistory(dealID)
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
	dataScope, scopeErr := resolveDealScope(userID, roleID, s.UserRepo)
	if scopeErr != nil {
		return scopeErr
	}
	if !dealMatchesScope(dataScope, deal) {
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
	dataScope, scopeErr := resolveDealScope(userID, roleID, s.UserRepo)
	if scopeErr != nil {
		return scopeErr
	}
	if !dealMatchesScope(dataScope, deal) {
		return ErrForbidden
	}
	if !deal.IsArchived {
		return ErrNotArchived
	}
	return s.Repo.Unarchive(id)
}
