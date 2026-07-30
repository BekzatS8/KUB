package services

import (
	"context"
	"errors"
	"log"
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
	StageRepo *repositories.FunnelStageRepository
	BoardNotifier BoardNotifier
	TransitionRuleRepo *repositories.FunnelTransitionRuleRepository
}

// SetStageRepo подключает репозиторий этапов воронки — нужен для авто-архива
// лида при переходе на этап, помеченный как «отправлять в архив».
func (s *LeadService) SetStageRepo(stageRepo *repositories.FunnelStageRepository) {
	s.StageRepo = stageRepo
}

// SetBoardNotifier подключает real-time рассыльщик обновлений доски (WebSocket).
func (s *LeadService) SetBoardNotifier(n BoardNotifier) {
	s.BoardNotifier = n
}

// SetTransitionRuleRepo подключает репозиторий правил перехода между воронками —
// чтобы лиды (а не только сделки) автоматически переходили в целевую воронку.
func (s *LeadService) SetTransitionRuleRepo(repo *repositories.FunnelTransitionRuleRepository) {
	s.TransitionRuleRepo = repo
}

// applyLeadTransitionRules применяет активные правила перехода к лиду по образцу
// сделок (DealService.applyTransitionRules). Если для (funnelID, stageID) есть
// активное правило — лид уходит в целевую воронку на целевой этап; рекурсивно
// проверяем и целевой этап. Возвращает true, если лид был перенесён правилом.
func (s *LeadService) applyLeadTransitionRules(leadID, funnelID, stageID, depth int) bool {
	if depth > 10 || s.TransitionRuleRepo == nil || s.StageRepo == nil || funnelID <= 0 {
		log.Printf("[lead_transition] skip lead=%d funnel=%d stage=%d (ruleRepo=%v stageRepo=%v depth=%d)",
			leadID, funnelID, stageID, s.TransitionRuleRepo != nil, s.StageRepo != nil, depth)
		return false
	}
	rules, err := s.TransitionRuleRepo.FindActiveByTrigger(funnelID, stageID)
	if err != nil {
		log.Printf("[lead_transition] lead=%d find rules error: %v", leadID, err)
		return false
	}
	if len(rules) == 0 {
		log.Printf("[lead_transition] lead=%d: НЕТ активного правила для funnel=%d stage=%d — переход не выполнен",
			leadID, funnelID, stageID)
		return false
	}
	rule := rules[0]
	toStage, err := s.StageRepo.GetByID(rule.ToStageID)
	if err != nil || toStage == nil {
		log.Printf("[lead_transition] lead=%d rule=%q target stage %d not found", leadID, rule.Name, rule.ToStageID)
		return false
	}
	if err := s.Repo.MoveStageAndFunnel(leadID, rule.ToStageID, rule.ToFunnelID); err != nil {
		log.Printf("[lead_transition] lead=%d rule=%q move failed: %v", leadID, rule.Name, err)
		return false
	}
	log.Printf("[lead_transition] lead=%d: правило %q → воронка=%d этап=%d ✓", leadID, rule.Name, rule.ToFunnelID, rule.ToStageID)
	if s.BoardNotifier != nil {
		s.BoardNotifier.NotifyBoardChanged(funnelID)
		s.BoardNotifier.NotifyBoardChanged(rule.ToFunnelID)
	}
	// Целевой этап тоже может иметь правило перехода.
	s.applyLeadTransitionRules(leadID, rule.ToFunnelID, rule.ToStageID, depth+1)
	return true
}

func NewLeadService(leadRepo *repositories.LeadRepository, dealRepo *repositories.DealRepository, clientRepo *repositories.ClientRepository, userRepo ...repositories.UserRepository) *LeadService {
	var clientSvc *ClientService
	if clientRepo != nil {
		clientSvc = NewClientService(clientRepo)
	}
	svc := &LeadService{Repo: leadRepo, DealRepo: dealRepo, ClientSvc: clientSvc}
	if len(userRepo) > 0 {
		svc.UserRepo = userRepo[0]
		if svc.ClientSvc != nil {
			svc.ClientSvc.SetUserRepo(userRepo[0])
		}
	}
	return svc
}

func (s *LeadService) Create(lead *models.Leads, userID, roleID int) (int64, error) {
	if authz.IsReadOnly(roleID) {
		return 0, ErrReadOnly
	}
	scope, err := resolveLeadScope(userID, roleID, s.UserRepo)
	if err != nil {
		return 0, err
	}
	switch scope.Kind {
	case ScopeKindOwn:
		lead.OwnerID = userID
	case ScopeKindBranch:
		// bind to user's branch so the new lead stays within the department scope
		if scope.BranchID != nil {
			lead.BranchID = scope.BranchID
		}
	}
	if lead.OwnerID == 0 {
		lead.OwnerID = userID
	}
	// sales must not set a different owner
	if roleID == authz.RoleSales && lead.OwnerID != userID {
		return 0, ErrForbidden
	}
	if !leadMatchesScope(scope, lead) {
		return 0, ErrForbidden
	}
	if lead.Status == "" {
		lead.Status = "new"
	}
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
	scope, err := resolveLeadScope(userID, roleID, s.UserRepo)
	if err != nil {
		return err
	}
	if !leadMatchesScope(scope, current) {
		return ErrForbidden
	}
	if roleID == authz.RoleSales && current.OwnerID != userID {
		return ErrForbidden
	}
	if roleID != authz.RoleManagement && roleID != authz.RoleSystemAdmin {
		lead.OwnerID = current.OwnerID
	} else {
		if lead.OwnerID == 0 {
			lead.OwnerID = current.OwnerID
		}
	}
	if lead.Status == "" {
		lead.Status = current.Status
	} else if lead.Status != current.Status {
		return errors.New("status must be updated via /leads/:id/status")
	}
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

func (s *LeadService) ListForRole(userID, roleID, limit, offset int, archiveScope repositories.ArchiveScope, filter repositories.LeadListFilter) ([]*models.Leads, error) {
	// sales/visa/partner resolve to ScopeKindBranch (свой отдел+филиал по воронкам)
	// via resolveLeadScope — no hard guard here. Was previously blocked, which
	// forced sales onto /leads/my (ListAll = все лиды), leaking every department.
	dataScope, err := resolveLeadScope(userID, roleID, s.UserRepo)
	if err != nil {
		return nil, err
	}
	return listLeadsForScope(s.Repo, dataScope, limit, offset, filter, archiveScope)
}

func (s *LeadService) ListMyWithFilterAndArchiveScope(ownerID int, limit, offset int, scope repositories.ArchiveScope, filter repositories.LeadListFilter) ([]*models.Leads, error) {
	// "Мои" = лиды, где текущий пользователь является владельцем (owner_id).
	// Ранее ошибочно вызывался ListAll → возвращались ВСЕ лиды компании.
	return s.Repo.ListByOwnerWithFilterAndArchiveScope(ownerID, limit, offset, filter, scope)
}

func (s *LeadService) ListForRoleWithTotal(userID, roleID, limit, offset int, archiveScope repositories.ArchiveScope, filter repositories.LeadListFilter) ([]*models.Leads, int, error) {
	items, err := s.ListForRole(userID, roleID, limit, offset, archiveScope, filter)
	if err != nil {
		return nil, 0, err
	}
	dataScope, err := resolveLeadScope(userID, roleID, s.UserRepo)
	if err != nil {
		return nil, 0, err
	}
	total, err := countLeadsForScope(s.Repo, dataScope, filter, archiveScope)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *LeadService) ListMyWithFilterAndArchiveScopeAndTotal(ownerID int, limit, offset int, scope repositories.ArchiveScope, filter repositories.LeadListFilter) ([]*models.Leads, int, error) {
	// "Мои" = лиды текущего владельца. Ранее возвращало ВСЕ лиды (ListAll).
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
	scope, err := resolveLeadScope(userID, roleID, s.UserRepo)
	if err != nil {
		return nil, err
	}
	if !leadMatchesScope(scope, lead) {
		return nil, ErrForbidden
	}
	return lead, nil
}

func (s *LeadService) GetByIDWithArchiveScope(id int, userID, roleID int, scope repositories.ArchiveScope) (*models.Leads, error) {
	lead, err := s.Repo.GetByIDWithArchiveScope(id, scope)
	if err != nil || lead == nil {
		return lead, err
	}
	dataScope, err := resolveLeadScope(userID, roleID, s.UserRepo)
	if err != nil {
		return nil, err
	}
	if !leadMatchesScope(dataScope, lead) {
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
	scope, err := resolveLeadScope(userID, roleID, s.UserRepo)
	if err != nil {
		return err
	}
	if !leadMatchesScope(scope, lead) {
		return ErrForbidden
	}
	// мягкое удаление в корзину с фиксацией автора (ТЗ п.7.1)
	return s.Repo.SoftDelete(id, userID)
}

// RestoreLead возвращает лид из корзины (только админ).
func (s *LeadService) RestoreLead(id, userID, roleID int) error {
	if !authz.CanHardDeleteBusinessEntity(roleID) {
		return ErrForbidden
	}
	return s.Repo.Restore(id)
}

// PurgeLead окончательно удаляет лид из корзины (только админ).
func (s *LeadService) PurgeLead(id, userID, roleID int) error {
	if !authz.CanHardDeleteBusinessEntity(roleID) {
		return ErrForbidden
	}
	return s.Repo.Purge(id)
}

// buildConvertedDeal assembles the Deal produced when converting a lead.
// branch_id is INHERITED from the source lead, symmetric with DealService.Create
// (deal_service.go: deal.BranchID = lead.BranchID). A lead with no branch yields a
// nil branch — no synthetic branch is invented; we only remove the artificial loss.
func buildConvertedDeal(leadID, clientID int, clientType string, ownerID int, amount float64, currency string, lead *models.Leads, createdAt time.Time) *models.Deals {
	return &models.Deals{
		LeadID:     leadID,
		ClientID:   clientID,
		ClientType: clientType,
		OwnerID:    ownerID,
		Amount:     amount,
		Currency:   currency,
		Status:     "new",
		CreatedAt:  createdAt,
		BranchID:   lead.BranchID,
		// the deal replaces the lead card on the kanban board, so it inherits
		// the lead's funnel position (ТЗ 04.07.2026, п.1.1)
		FunnelID: lead.FunnelID,
		StageID:  lead.StageID,
	}
}

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
	scope, err := resolveLeadScope(userID, roleID, s.UserRepo)
	if err != nil {
		return nil, err
	}
	if roleID == authz.RoleSales && lead.OwnerID != userID {
		return nil, ErrForbidden
	}
	if !leadMatchesScope(scope, lead) {
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
	deal := buildConvertedDeal(leadID, clientID, normalizedClientType, ownerID, amount, currency, lead, time.Now())
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
	scope, err := resolveLeadScope(userID, roleID, s.UserRepo)
	if err != nil {
		return err
	}
	if roleID == authz.RoleSales && lead.OwnerID != userID {
		return ErrForbidden
	}
	if !leadMatchesScope(scope, lead) {
		return ErrForbidden
	}
	if !canTransition(lead.Status, to, LeadTransitions) {
		return errors.New("invalid status transition")
	}
	return s.Repo.UpdateStatus(id, to)
}

// MoveToStage moves a lead across kanban stages (ТЗ 04.07.2026, п.1.1).
// Access is decided by scope alone: a manager works every lead of their branch
// and department, no matter whose account it currently sits on. Ownership was
// previously an extra gate here, which deadlocked the board — inbound leads
// arrive owned by the integration's admin account, so every card a manager
// dragged came back 403.
func (s *LeadService) MoveToStage(id, stageID, userID, roleID int) error {
	if authz.IsReadOnly(roleID) {
		return ErrReadOnly
	}
	if !authz.CanWorkWithLeads(roleID) {
		return ErrForbidden
	}
	lead, err := s.Repo.GetByID(id)
	if err != nil {
		return err
	}
	if lead == nil {
		return ErrLeadNotFound
	}
	scope, err := resolveLeadScope(userID, roleID, s.UserRepo)
	if err != nil {
		return err
	}
	if !leadMatchesScope(scope, lead) {
		return ErrForbidden
	}
	if err := s.Repo.MoveStage(id, stageID, userID, s.claimsOwnershipOnMove(lead, userID, roleID)); err != nil {
		return err
	}
	targetFunnelID := 0
	autoArchive := false
	isRejection := false
	if s.StageRepo != nil {
		if stage, err := s.StageRepo.GetByID(stageID); err == nil && stage != nil {
			targetFunnelID = stage.FunnelID
			autoArchive = stage.AutoArchive
			isRejection = stage.IsRejection
		}
	}

	// Правила перехода между воронками — теперь и для лидов, не только сделок.
	// Ключевой сценарий «Передача клиента в Визовый отдел»: лид на этапе
	// «Договор подписан» автоматически уходит в целевую воронку. Раньше правила
	// срабатывали только у сделок, и лид просто оставался на месте.
	ruleMoved := s.applyLeadTransitionRules(id, targetFunnelID, stageID, 1)

	// Этап отказа: лид получает статус «отказ» (cancelled) — уходит из активной
	// воронки и попадает в «Отказники». Только если правило перехода не увело
	// лид в другую воронку.
	if !ruleMoved && isRejection {
		if err := s.Repo.UpdateStatus(id, "cancelled"); err != nil {
			return err
		}
	}

	// Авто-архив финального этапа — только если правило перехода НЕ увело лид
	// в другую воронку и это НЕ этап отказа (отказ уходит в Отказники, не в Архив).
	if !ruleMoved && autoArchive && !isRejection && !lead.IsArchived {
		if err := s.Repo.Archive(id, userID, "Этап завершён (автоархив)"); err != nil {
			return err
		}
	}

	// Real-time: перечитать доску исходной и целевой воронки. Если сработало
	// правило перехода, оно уже уведомило свои воронки (лишние сигналы
	// безвредны — на клиенте дебаунс).
	if s.BoardNotifier != nil {
		if lead.FunnelID != nil {
			s.BoardNotifier.NotifyBoardChanged(*lead.FunnelID)
		}
		if targetFunnelID > 0 && (lead.FunnelID == nil || *lead.FunnelID != targetFunnelID) {
			s.BoardNotifier.NotifyBoardChanged(targetFunnelID)
		}
	}
	return nil
}

// claimsOwnershipOnMove reports whether moving the card also hands the lead to
// the actor. A manager taking a card off the board becomes responsible for it
// when nobody is really working it yet — either it has no owner, or it is still
// parked on an admin/management account, which is how Instagram/WhatsApp leads
// land in the system. A lead already owned by another working manager keeps its
// owner: dragging a colleague's card must not silently take it from them.
// Admin/management move cards without taking them over.
func (s *LeadService) claimsOwnershipOnMove(lead *models.Leads, userID, roleID int) bool {
	if authz.IsElevated(roleID) {
		return false
	}
	if lead.OwnerID == 0 {
		return true
	}
	if lead.OwnerID == userID {
		return false
	}
	if s.UserRepo == nil {
		return false
	}
	owner, err := s.UserRepo.GetByID(lead.OwnerID)
	if err != nil || owner == nil {
		return false
	}
	return authz.IsElevated(owner.RoleID)
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
	scope, err := resolveLeadScope(userID, roleID, s.UserRepo)
	if err != nil {
		return err
	}
	if roleID == authz.RoleSales && lead.OwnerID != userID {
		return ErrForbidden
	}
	if !leadMatchesScope(scope, lead) {
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
	scope, err := resolveLeadScope(userID, roleID, s.UserRepo)
	if err != nil {
		return err
	}
	if roleID == authz.RoleSales && lead.OwnerID != userID {
		return ErrForbidden
	}
	if !leadMatchesScope(scope, lead) {
		return ErrForbidden
	}
	if !lead.IsArchived {
		return ErrNotArchived
	}
	return s.Repo.Unarchive(id)
}

func (s *LeadService) AssignOwner(id, assigneeID, userID, roleID int) error {
	if roleID != authz.RoleManagement && roleID != authz.RoleSystemAdmin {
		return ErrForbidden
	}
	return s.Repo.UpdateOwner(id, assigneeID)
}
