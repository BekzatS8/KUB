package services

import (
	"database/sql"
	"errors"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type PermissionService struct {
	repo *repositories.PermissionRepository
}

func NewPermissionService(repo *repositories.PermissionRepository) *PermissionService {
	return &PermissionService{repo: repo}
}

func (s *PermissionService) GetMe(userID int) (*models.PermissionsMeResponse, error) {
	p, err := s.repo.GetPrincipal(userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	roleCode := p.RoleCode
	if roleCode == "" {
		roleCode = authz.RoleCodeByID(p.RoleID)
	}

	assignments, err := s.repo.ListRolePermissions(p.RoleID)
	if err != nil {
		return nil, err
	}
	if len(assignments) == 0 {
		assignments = authz.PermissionAssignmentsForRole(roleCode)
	}
	perms := make([]authz.Permission, 0, len(assignments))
	for _, assignment := range assignments {
		perms = append(perms, authz.Permission{Action: assignment.Action, Scope: assignment.Scope})
	}
	scopes := authz.PermissionScopes(perms)

	return &models.PermissionsMeResponse{
		Role: map[string]any{
			"id":          p.RoleID,
			"name":        p.RoleName,
			"code":        roleCode,
			"legacy_code": authz.RoleCodeByID(p.RoleID),
		},
		Department:   p.Department,
		Branch:       p.Branch,
		Permissions:  assignments,
		Scopes:       scopes,
		MenuSections: authz.MenuSectionsForScopes(scopes),
	}, nil
}
