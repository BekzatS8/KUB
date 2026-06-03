package services

import (
	"database/sql"
	"errors"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type RoleService interface {
	CreateRole(role *models.Role) error
	GetRoleByID(id int) (*models.Role, error)
	UpdateRole(role *models.Role) error
	DeleteRole(id int) error
	ListRoles(limit, offset int) ([]*models.Role, error)
	GetRoleCount() (int, error)
	GetRolesWithUserCounts() ([]map[string]interface{}, error)
}

type roleService struct {
	repo repositories.RoleRepository
}

func NewRoleService(repo repositories.RoleRepository) RoleService {
	return &roleService{repo: repo}
}

func (s *roleService) CreateRole(role *models.Role) error {
	return s.repo.Create(role)
}

func (s *roleService) GetRoleByID(id int) (*models.Role, error) {
	return s.repo.GetByID(id)
}

func (s *roleService) UpdateRole(role *models.Role) error {
	if err := s.repo.Update(role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *roleService) DeleteRole(id int) error {
	if err := s.repo.Delete(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if repositories.IsSQLState(err, repositories.SQLStateForeignKey) {
			return ErrRoleInUse
		}
		return err
	}
	return nil
}

func (s *roleService) ListRoles(limit, offset int) ([]*models.Role, error) {
	return s.repo.List(limit, offset)
}

func (s *roleService) GetRoleCount() (int, error) {
	return s.repo.GetCount()
}

func (s *roleService) GetRolesWithUserCounts() ([]map[string]interface{}, error) {
	return s.repo.GetRolesWithUserCounts()
}
