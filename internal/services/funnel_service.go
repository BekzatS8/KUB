package services

import (
	"database/sql"
	"errors"
	"strings"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type FunnelService struct {
	repo           *repositories.FunnelRepository
	permissionRepo *repositories.PermissionRepository
}

func NewFunnelService(repo *repositories.FunnelRepository, permissionRepo *repositories.PermissionRepository) *FunnelService {
	return &FunnelService{repo: repo, permissionRepo: permissionRepo}
}

func (s *FunnelService) principal(userID int) (*models.PermissionPrincipal, error) {
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

func (s *FunnelService) List(userID int) ([]*models.Funnel, error) {
	p, err := s.principal(userID)
	if err != nil {
		return nil, err
	}
	if !authz.HasPermission(p.RoleCode, authz.ActionFunnelsView) {
		return nil, ErrForbidden
	}

	switch p.RoleCode {
	case "admin":
		return s.repo.List(repositories.FunnelListFilter{})
	case "management", "quality_control":
		funnels, err := s.repo.List(repositories.FunnelListFilter{ActiveOnly: true})
		if err != nil {
			return nil, err
		}
		return filterFunnelsByDepartments(funnels, map[string]struct{}{"sales": {}, "visa": {}, "partner": {}}), nil
	case "sales", "visa", "partner":
		return s.repo.List(repositories.FunnelListFilter{DepartmentCode: p.RoleCode, BranchID: p.BranchID, ActiveOnly: true})
	default:
		return []*models.Funnel{}, nil
	}
}

func filterFunnelsByDepartments(funnels []*models.Funnel, allowed map[string]struct{}) []*models.Funnel {
	out := make([]*models.Funnel, 0, len(funnels))
	for _, f := range funnels {
		if f.Department == nil {
			continue
		}
		if _, ok := allowed[f.Department.Code]; ok {
			out = append(out, f)
		}
	}
	return out
}

func (s *FunnelService) GetByID(id, userID int) (*models.Funnel, error) {
	f, err := s.repo.GetByID(id)
	if err != nil || f == nil {
		return f, err
	}
	p, err := s.principal(userID)
	if err != nil {
		return nil, err
	}
	if !s.canViewFunnel(p, f) {
		return nil, ErrForbidden
	}
	return f, nil
}

func (s *FunnelService) canViewFunnel(p *models.PermissionPrincipal, f *models.Funnel) bool {
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

func (s *FunnelService) canMoveLead(p *models.PermissionPrincipal, lead *repositories.LeadFunnelAccess) bool {
	if p == nil || lead == nil {
		return false
	}
	switch p.RoleCode {
	case "admin", "management":
		return true
	default:
		return false
	}
}

func (s *FunnelService) Create(f *models.Funnel, userID int) error {
	p, err := s.principal(userID)
	if err != nil {
		return err
	}
	if !authz.HasPermission(p.RoleCode, authz.ActionFunnelsCreate) {
		return ErrForbidden
	}
	normalizeFunnel(f)
	f.CreatedBy = &userID
	return s.repo.Create(f)
}

func (s *FunnelService) Update(f *models.Funnel, userID int) error {
	p, err := s.principal(userID)
	if err != nil {
		return err
	}
	if !authz.HasPermission(p.RoleCode, authz.ActionFunnelsUpdate) {
		return ErrForbidden
	}
	normalizeFunnel(f)
	if err := s.repo.Update(f); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func normalizeFunnel(f *models.Funnel) {
	f.Name = strings.TrimSpace(f.Name)
	f.Code = strings.TrimSpace(strings.ToLower(f.Code))
	if !f.IsActive {
		return
	}
}

func (s *FunnelService) Delete(id, userID int) error {
	p, err := s.principal(userID)
	if err != nil {
		return err
	}
	if !authz.HasPermission(p.RoleCode, authz.ActionFunnelsDelete) {
		return ErrForbidden
	}
	if err := s.repo.Delete(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *FunnelService) Reorder(ids []int, userID int) error {
	p, err := s.principal(userID)
	if err != nil {
		return err
	}
	if !authz.HasPermission(p.RoleCode, authz.ActionFunnelsReorder) {
		return ErrForbidden
	}
	return s.repo.Reorder(ids)
}

func (s *FunnelService) MoveLeadToFunnel(leadID, funnelID, userID int) error {
	p, err := s.principal(userID)
	if err != nil {
		return err
	}
	if !authz.HasPermission(p.RoleCode, authz.ActionLeadsMoveBetweenFunnels) {
		return ErrForbidden
	}
	f, err := s.repo.GetByID(funnelID)
	if err != nil {
		return err
	}
	if f == nil {
		return ErrNotFound
	}
	if !s.canViewFunnel(p, f) {
		return ErrForbidden
	}
	lead, err := s.repo.GetLeadFunnelAccess(leadID)
	if err != nil {
		return err
	}
	if lead == nil {
		return ErrNotFound
	}
	if !s.canMoveLead(p, lead) {
		return ErrForbidden
	}
	if err := s.repo.MoveLeadToFunnel(leadID, funnelID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}
