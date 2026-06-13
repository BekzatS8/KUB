package services

import (
	"database/sql"
	"errors"
	"strings"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type FunnelStageService struct {
	repo           *repositories.FunnelStageRepository
	funnelRepo     *repositories.FunnelRepository
	permissionRepo *repositories.PermissionRepository
	userRepo       repositories.UserRepository
}

func NewFunnelStageService(repo *repositories.FunnelStageRepository, funnelRepo *repositories.FunnelRepository, permissionRepo *repositories.PermissionRepository) *FunnelStageService {
	return &FunnelStageService{repo: repo, funnelRepo: funnelRepo, permissionRepo: permissionRepo}
}

func (s *FunnelStageService) SetUserRepo(userRepo repositories.UserRepository) {
	s.userRepo = userRepo
}

func (s *FunnelStageService) principal(userID int) (*models.PermissionPrincipal, error) {
	p, err := s.permissionRepo.GetPrincipal(userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if p.RoleCode == "" {
		p.RoleCode = authz.RoleCodeByID(p.RoleID)
	}
	return p, nil
}

// canViewFunnel mirrors FunnelService.canViewFunnel: department-scoped visibility
// per the access matrix (admin: all; management/quality_control: sales+visa+partner;
// sales/visa/partner: own department only).
func (s *FunnelStageService) canViewFunnel(p *models.PermissionPrincipal, f *models.Funnel) bool {
	if !authz.HasPermission(p.RoleCode, authz.ActionFunnelsView) || f == nil || f.Department == nil {
		return false
	}
	switch p.RoleCode {
	case "admin":
		return true
	case "management", "quality_control":
		switch f.Department.Code {
		case "sales", "visa", "partner":
			return true
		default:
			return false
		}
	case "sales", "visa", "partner":
		if f.Department.Code != p.RoleCode {
			return false
		}
		return f.BranchID == nil || p.BranchID == nil || *f.BranchID == *p.BranchID
	default:
		return false
	}
}

func (s *FunnelStageService) loadFunnelForView(funnelID int, p *models.PermissionPrincipal) (*models.Funnel, error) {
	f, err := s.funnelRepo.GetByID(funnelID)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, ErrNotFound
	}
	if !s.canViewFunnel(p, f) {
		return nil, ErrForbidden
	}
	return f, nil
}

func (s *FunnelStageService) loadFunnelForManage(funnelID int, p *models.PermissionPrincipal, action string) (*models.Funnel, error) {
	if !authz.HasPermission(p.RoleCode, action) {
		return nil, ErrForbidden
	}
	return s.loadFunnelForView(funnelID, p)
}

func (s *FunnelStageService) ListStages(funnelID, userID int) ([]*models.FunnelStage, error) {
	p, err := s.principal(userID)
	if err != nil {
		return nil, err
	}
	if _, err := s.loadFunnelForView(funnelID, p); err != nil {
		return nil, err
	}
	return s.repo.ListByFunnel(funnelID)
}

func normalizeStage(s *models.FunnelStage) {
	s.Name = strings.TrimSpace(s.Name)
	s.Code = strings.TrimSpace(strings.ToLower(s.Code))
	if s.Color == "" {
		s.Color = "#94a3b8"
	}
	switch s.Type {
	case models.FunnelStageTypeWon, models.FunnelStageTypeLost:
	default:
		s.Type = models.FunnelStageTypeRegular
	}
	if s.Probability < 0 {
		s.Probability = 0
	}
	if s.Probability > 100 {
		s.Probability = 100
	}
}

func (s *FunnelStageService) CreateStage(stage *models.FunnelStage, userID int) error {
	p, err := s.principal(userID)
	if err != nil {
		return err
	}
	if _, err := s.loadFunnelForManage(stage.FunnelID, p, authz.ActionFunnelsUpdate); err != nil {
		return err
	}
	normalizeStage(stage)
	stage.IsActive = true
	return s.repo.Create(stage)
}

func (s *FunnelStageService) UpdateStage(stage *models.FunnelStage, userID int) error {
	p, err := s.principal(userID)
	if err != nil {
		return err
	}
	existing, err := s.repo.GetByID(stage.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrNotFound
	}
	if _, err := s.loadFunnelForManage(existing.FunnelID, p, authz.ActionFunnelsUpdate); err != nil {
		return err
	}
	stage.FunnelID = existing.FunnelID
	normalizeStage(stage)
	if err := s.repo.Update(stage); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *FunnelStageService) DeleteStage(id int, reassignToStageID *int, userID int) error {
	p, err := s.principal(userID)
	if err != nil {
		return err
	}
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrNotFound
	}
	if _, err := s.loadFunnelForManage(existing.FunnelID, p, authz.ActionFunnelsDelete); err != nil {
		return err
	}
	if reassignToStageID != nil {
		target, err := s.repo.GetByID(*reassignToStageID)
		if err != nil {
			return err
		}
		if target == nil || target.FunnelID != existing.FunnelID {
			return ErrInvalidState
		}
	}
	if err := s.repo.Delete(id, reassignToStageID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if errors.Is(err, repositories.ErrStageHasDeals) {
			return ErrStageHasDeals
		}
		return err
	}
	return nil
}

func (s *FunnelStageService) ReorderStages(funnelID int, ids []int, userID int) error {
	p, err := s.principal(userID)
	if err != nil {
		return err
	}
	if _, err := s.loadFunnelForManage(funnelID, p, authz.ActionFunnelsReorder); err != nil {
		return err
	}
	return s.repo.Reorder(funnelID, ids)
}

func (s *FunnelStageService) DuplicateStage(id, userID int) (*models.FunnelStage, error) {
	p, err := s.principal(userID)
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrNotFound
	}
	if _, err := s.loadFunnelForManage(existing.FunnelID, p, authz.ActionFunnelsCreate); err != nil {
		return nil, err
	}
	return s.repo.Duplicate(id)
}

func (s *FunnelStageService) Board(funnelID, userID int) (*models.FunnelBoard, error) {
	p, err := s.principal(userID)
	if err != nil {
		return nil, err
	}
	funnel, err := s.loadFunnelForView(funnelID, p)
	if err != nil {
		return nil, err
	}
	stages, err := s.repo.ListByFunnel(funnelID)
	if err != nil {
		return nil, err
	}

	var deals []*models.FunnelBoardDeal
	dataScope, scopeErr := resolveDealScope(userID, p.RoleID, s.userRepo)
	if scopeErr == nil && dataScope.Kind != ScopeKindForbidden {
		var branchID, deptID *int
		if dataScope.Kind == ScopeKindBranch {
			branchID = dataScope.BranchID
			deptID = dataScope.DepartmentID
		}
		deals, err = s.repo.ListBoardDeals(funnelID, branchID, deptID)
		if err != nil {
			return nil, err
		}
	}

	byStage := map[int][]*models.FunnelBoardDeal{}
	unassigned := []*models.FunnelBoardDeal{}
	for _, d := range deals {
		if d.StageID == nil {
			unassigned = append(unassigned, d)
			continue
		}
		byStage[*d.StageID] = append(byStage[*d.StageID], d)
	}

	columns := make([]*models.FunnelBoardColumn, 0, len(stages))
	for _, st := range stages {
		colDeals := byStage[st.ID]
		if colDeals == nil {
			colDeals = []*models.FunnelBoardDeal{}
		}
		total := 0.0
		for _, d := range colDeals {
			total += d.Amount
		}
		columns = append(columns, &models.FunnelBoardColumn{
			Stage:       st,
			Deals:       colDeals,
			Count:       len(colDeals),
			TotalAmount: total,
		})
	}

	if len(unassigned) > 0 {
		total := 0.0
		for _, d := range unassigned {
			total += d.Amount
		}
		columns = append([]*models.FunnelBoardColumn{{
			Stage:       nil,
			Deals:       unassigned,
			Count:       len(unassigned),
			TotalAmount: total,
		}}, columns...)
	}

	return &models.FunnelBoard{Funnel: funnel, Columns: columns}, nil
}
