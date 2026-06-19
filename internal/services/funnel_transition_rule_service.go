package services

import (
	"database/sql"
	"errors"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type FunnelTransitionRuleService struct {
	repo      *repositories.FunnelTransitionRuleRepository
	stageRepo *repositories.FunnelStageRepository
}

func NewFunnelTransitionRuleService(
	repo *repositories.FunnelTransitionRuleRepository,
	stageRepo *repositories.FunnelStageRepository,
) *FunnelTransitionRuleService {
	return &FunnelTransitionRuleService{repo: repo, stageRepo: stageRepo}
}

func (s *FunnelTransitionRuleService) List() ([]*models.FunnelTransitionRule, error) {
	return s.repo.ListEnriched()
}

func (s *FunnelTransitionRuleService) GetByID(id int) (*models.FunnelTransitionRule, error) {
	rule, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, ErrNotFound
	}
	return rule, nil
}

func (s *FunnelTransitionRuleService) Create(rule *models.FunnelTransitionRule, roleID int) error {
	if !authz.CanManageFunnels(roleID) {
		return ErrForbidden
	}
	if rule.FromFunnelID == 0 || rule.FromStageID == 0 || rule.ToFunnelID == 0 || rule.ToStageID == 0 {
		return ErrInvalidState
	}
	if rule.FromStageID == rule.ToStageID && rule.FromFunnelID == rule.ToFunnelID {
		return ErrInvalidState
	}
	// Validate that the stages belong to the declared funnels.
	fromStage, err := s.stageRepo.GetByID(rule.FromStageID)
	if err != nil || fromStage == nil || fromStage.FunnelID != rule.FromFunnelID {
		return ErrInvalidState
	}
	toStage, err := s.stageRepo.GetByID(rule.ToStageID)
	if err != nil || toStage == nil || toStage.FunnelID != rule.ToFunnelID {
		return ErrInvalidState
	}
	return s.repo.Create(rule)
}

func (s *FunnelTransitionRuleService) Update(rule *models.FunnelTransitionRule, roleID int) error {
	if !authz.CanManageFunnels(roleID) {
		return ErrForbidden
	}
	if rule.FromFunnelID == 0 || rule.FromStageID == 0 || rule.ToFunnelID == 0 || rule.ToStageID == 0 {
		return ErrInvalidState
	}
	existing, err := s.repo.GetByID(rule.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrNotFound
	}
	fromStage, err := s.stageRepo.GetByID(rule.FromStageID)
	if err != nil || fromStage == nil || fromStage.FunnelID != rule.FromFunnelID {
		return ErrInvalidState
	}
	toStage, err := s.stageRepo.GetByID(rule.ToStageID)
	if err != nil || toStage == nil || toStage.FunnelID != rule.ToFunnelID {
		return ErrInvalidState
	}
	err = s.repo.Update(rule)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *FunnelTransitionRuleService) Delete(id, roleID int) error {
	if !authz.CanManageFunnels(roleID) {
		return ErrForbidden
	}
	err := s.repo.Delete(id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// ToggleActive flips the is_active flag of a rule.
func (s *FunnelTransitionRuleService) ToggleActive(id int, active bool, roleID int) error {
	if !authz.CanManageFunnels(roleID) {
		return ErrForbidden
	}
	rule, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if rule == nil {
		return ErrNotFound
	}
	rule.IsActive = active
	return s.repo.Update(rule)
}
