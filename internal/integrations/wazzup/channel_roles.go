package wazzup

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

// Доступ сотрудников к чатам номера — аналог окна «Выбор ролей» в кабинете
// Wazzup. Нужен дочернему White Label аккаунту: своего кабинета у него нет,
// и роли на его номерах выдаёт CRM (см. syncUserRoles).

// ErrRolesManagedInCabinet — номер обычного аккаунта: его роли настраиваются
// в кабинете Wazzup, CRM их не трогает.
var ErrRolesManagedInCabinet = fmt.Errorf("%w: доступ к номерам этого аккаунта настраивается в кабинете Wazzup", ErrBadRequest)

// ChannelRoleItem — строка окна «Доступ к чатам»: сотрудник и его роль на
// номере. Role пустая — доступа нет.
type ChannelRoleItem struct {
	UserID             int    `json:"user_id"`
	Name               string `json:"name"`
	CRMRoleID          int    `json:"crm_role_id"`
	BranchName         string `json:"branch_name,omitempty"`
	Role               string `json:"role"`
	AllowGetNewClients bool   `json:"allow_get_new_clients"`
}

// ChannelRolesView — доступ к номеру. Configured=false — роли сейчас
// автоматические, и Items показывает, какие именно.
type ChannelRolesView struct {
	ChannelID  int64             `json:"channel_id"`
	Configured bool              `json:"configured"`
	Items      []ChannelRoleItem `json:"items"`
}

// ChannelRoleInput — роль сотрудника в запросе на сохранение.
type ChannelRoleInput struct {
	UserID             int    `json:"user_id"`
	Role               string `json:"role"`
	AllowGetNewClients bool   `json:"allow_get_new_clients"`
}

func validChannelRole(role string) bool {
	switch role {
	case "seller", "manager", "auditor":
		return true
	}
	return false
}

// partnerChannel находит номер и его аккаунт; доступ настраивается только у
// аккаунта без своего кабинета (White Label).
func (s *Service) partnerChannel(ctx context.Context, channelID int64) (*AccountConfig, *models.WazzupIntegration, *models.WazzupChannel, error) {
	acc, integration, channel, err := s.accountOfChannelRow(ctx, channelID)
	if err != nil {
		return nil, nil, nil, err
	}
	if acc == nil || channel == nil {
		return nil, nil, nil, sql.ErrNoRows
	}
	if !acc.Partner {
		return nil, nil, nil, ErrRolesManagedInCabinet
	}
	return acc, integration, channel, nil
}

// ChannelRoles — текущий доступ сотрудников к номеру.
func (s *Service) ChannelRoles(ctx context.Context, channelID int64) (*ChannelRolesView, error) {
	_, _, channel, err := s.partnerChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	candidates, err := s.repo.ListRoleCandidates(ctx)
	if err != nil {
		return nil, err
	}
	manual := map[int]repositories.ChannelUserRole{}
	if channel.RolesConfigured {
		stored, err := s.repo.ListChannelRoles(ctx, channelID)
		if err != nil {
			return nil, err
		}
		for _, r := range stored {
			manual[r.UserID] = r
		}
	}

	view := &ChannelRolesView{ChannelID: channelID, Configured: channel.RolesConfigured, Items: make([]ChannelRoleItem, 0, len(candidates))}
	for _, c := range candidates {
		item := ChannelRoleItem{UserID: c.ID, Name: c.Name, CRMRoleID: c.RoleID, BranchName: c.BranchName}
		if channel.RolesConfigured {
			if r, ok := manual[c.ID]; ok {
				item.Role = r.Role
				item.AllowGetNewClients = r.AllowGetNewClients
			}
		} else {
			item.Role = wazzupRoleFor(c.RoleID)
			item.AllowGetNewClients = item.Role == "seller"
		}
		view.Items = append(view.Items, item)
	}
	return view, nil
}

// SetChannelRoles сохраняет доступ к номеру и сразу отправляет роли в Wazzup.
// Сотрудники без роли в запросе доступа к номеру не получают.
func (s *Service) SetChannelRoles(ctx context.Context, channelID int64, items []ChannelRoleInput) (*ChannelRolesView, error) {
	acc, integration, _, err := s.partnerChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	candidates, err := s.repo.ListRoleCandidates(ctx)
	if err != nil {
		return nil, err
	}
	known := make(map[int]bool, len(candidates))
	for _, c := range candidates {
		known[c.ID] = true
	}

	seen := map[int]bool{}
	roles := make([]repositories.ChannelUserRole, 0, len(items))
	for _, it := range items {
		role := strings.TrimSpace(it.Role)
		if role == "" {
			continue
		}
		if !validChannelRole(role) {
			return nil, fmt.Errorf("%w: неизвестная роль %q", ErrBadRequest, role)
		}
		if !known[it.UserID] {
			return nil, fmt.Errorf("%w: сотрудник %d не найден или заблокирован", ErrBadRequest, it.UserID)
		}
		if seen[it.UserID] {
			continue
		}
		seen[it.UserID] = true
		roles = append(roles, repositories.ChannelUserRole{UserID: it.UserID, Role: role, AllowGetNewClients: it.AllowGetNewClients})
	}

	if err := s.repo.ReplaceChannelRoles(ctx, channelID, roles); err != nil {
		return nil, err
	}
	if err := s.pushAccountRoles(ctx, acc, integration); err != nil {
		return nil, err
	}
	return s.ChannelRoles(ctx, channelID)
}

// ResetChannelRoles возвращает номеру автоматические роли по ролям CRM.
func (s *Service) ResetChannelRoles(ctx context.Context, channelID int64) (*ChannelRolesView, error) {
	acc, integration, _, err := s.partnerChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.ResetChannelRoles(ctx, channelID); err != nil {
		return nil, err
	}
	if err := s.pushAccountRoles(ctx, acc, integration); err != nil {
		return nil, err
	}
	return s.ChannelRoles(ctx, channelID)
}

// pushAccountRoles отправляет в Wazzup роли всех номеров аккаунта: провайдер
// заменяет набор ролей целиком.
func (s *Service) pushAccountRoles(ctx context.Context, acc *AccountConfig, integration *models.WazzupIntegration) error {
	channels, err := s.repo.ListChannels(ctx, integration.ID)
	if err != nil {
		return err
	}
	return s.syncUserRoles(ctx, acc, s.apiKeyFor(acc, integration), channels)
}
