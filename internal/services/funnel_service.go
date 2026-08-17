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
	// Руководство и контроль качества — надведомственные роли: видят воронки
	// всех бизнес-отделов (sales/visa/partner).
	case "management", "quality_control":
		funnels, err := s.repo.List(repositories.FunnelListFilter{ActiveOnly: true})
		if err != nil {
			return nil, err
		}
		return filterFunnelsByDepartments(funnels, businessFunnelDepartments), nil
	// Менеджер отдела видит ТОЛЬКО воронки своего отдела — визовый отдел не должен
	// видеть воронки продаж/партнёров и наоборот (обратная связь заказчика
	// 17.08.2026). Раньше sales/visa/partner были свалены в одну группу и каждый
	// видел все три отдела.
	case "sales", "visa", "partner":
		funnels, err := s.repo.List(repositories.FunnelListFilter{ActiveOnly: true})
		if err != nil {
			return nil, err
		}
		return filterFunnelsByDepartments(funnels, ownDepartmentSet(p)), nil
	default:
		return []*models.Funnel{}, nil
	}
}

// businessFunnelDepartments — отделы, у которых вообще есть воронки. Надведом-
// ственные роли (руководство, контроль качества) видят все их воронки.
var businessFunnelDepartments = map[string]struct{}{"sales": {}, "visa": {}, "partner": {}}

// ownDepartmentSet возвращает набор из одного отдела — того, к которому относится
// пользователь. Берём department_code принципала; если он пуст, откатываемся на
// код роли (для sales/visa/partner код роли совпадает с кодом отдела).
func ownDepartmentSet(p *models.PermissionPrincipal) map[string]struct{} {
	code := strings.TrimSpace(p.DepartmentCode)
	if code == "" {
		code = p.RoleCode
	}
	return map[string]struct{}{code: {}}
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
	// Руководство и контроль качества видят все бизнес-воронки.
	case "management", "quality_control":
		_, ok := businessFunnelDepartments[f.Department.Code]
		return ok
	// Менеджер отдела — только воронка своего отдела (обратная связь 17.08.2026).
	case "sales", "visa", "partner":
		_, ok := ownDepartmentSet(p)[f.Department.Code]
		return ok
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
