package wazzup

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"turcompany/internal/models"
)

// Несколько аккаунтов Wazzup одновременно.
//
// Основной аккаунт работает через User API по статическому apiKey, дочерний
// (White Label) — через Tech Partner API партнёрскими доступами. Раньше CRM
// была подключена к одному из них за раз (переключатель WAZZUP_DRIVER); теперь
// оба работают рядом: у каждого своё подключение в wazzup_integrations — свой
// webhook-токен, crmKey и каналы с привязкой к филиалам.
//
// Какой аккаунт нужен в конкретной операции, определяется по данным, а не по
// пользователю: отправка — по каналу, с которого пишут; переписка — по каналу
// чата; окно мессенджера — по выбранному пункту меню.

const (
	AccountMain  = "main"
	AccountChild = "child"
)

var (
	// ErrAccountNotConfigured — аккаунт не задан в конфиге (нет ключей).
	ErrAccountNotConfigured = fmt.Errorf("%w: wazzup account is not configured", ErrNotFound)
	// ErrAccountNotConnected — ключи есть, но подключение ещё не создано:
	// администратор не нажимал «Подключить» для этого аккаунта.
	ErrAccountNotConnected = fmt.Errorf("%w: wazzup account is not connected", ErrNotFound)
)

// AccountConfig — аккаунт Wazzup, доступный этой инсталляции.
type AccountConfig struct {
	Name   string // main | child
	Title  string // подпись в интерфейсе
	Client Client
	// APIKey — статический ключ User API. У дочернего аккаунта его нет:
	// авторизацию делает клиент партнёрскими доступами.
	APIKey string
	// Partner — аккаунт работает через Tech Partner API (White Label).
	Partner bool
}

// AccountInfo — состояние аккаунта для интерфейса.
type AccountInfo struct {
	Account    string `json:"account"`
	Title      string `json:"title"`
	Partner    bool   `json:"partner"`
	Connected  bool   `json:"connected"`
	Enabled    bool   `json:"enabled"`
	WebhookURL string `json:"webhook_url,omitempty"`
}

// RegisterAccount подключает аккаунт (или заменяет уже зарегистрированный с
// тем же именем). Основной всегда идёт первым — он аккаунт по умолчанию.
func (s *Service) RegisterAccount(cfg AccountConfig) {
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	if cfg.Name == "" || cfg.Client == nil {
		return
	}
	acc := &cfg
	for i, existing := range s.accounts {
		if existing.Name == acc.Name {
			s.accounts[i] = acc
			return
		}
	}
	if acc.Name == AccountMain {
		s.accounts = append([]*AccountConfig{acc}, s.accounts...)
		return
	}
	s.accounts = append(s.accounts, acc)
}

// accountByName — аккаунт по имени; пустое имя — аккаунт по умолчанию.
func (s *Service) accountByName(name string) (*AccountConfig, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		if len(s.accounts) == 0 {
			return nil, ErrAccountNotConfigured
		}
		return s.accounts[0], nil
	}
	for _, acc := range s.accounts {
		if acc.Name == name {
			return acc, nil
		}
	}
	return nil, ErrAccountNotConfigured
}

// connection — действующее подключение аккаунта.
func (s *Service) connection(ctx context.Context, acc *AccountConfig) (*models.WazzupIntegration, error) {
	integration, err := s.repo.GetIntegrationByAccount(ctx, acc.Name)
	if err != nil {
		return nil, err
	}
	if integration == nil {
		return nil, ErrAccountNotConnected
	}
	if !integration.Enabled {
		return nil, ErrDisabled
	}
	return integration, nil
}

// connectedAccount — аккаунт по имени вместе с его подключением.
func (s *Service) connectedAccount(ctx context.Context, name string) (*AccountConfig, *models.WazzupIntegration, error) {
	acc, err := s.accountByName(name)
	if err != nil {
		return nil, nil, err
	}
	integration, err := s.connection(ctx, acc)
	if err != nil {
		return nil, nil, err
	}
	return acc, integration, nil
}

// firstConnected — первый подключённый аккаунт (основной, если подключён).
func (s *Service) firstConnected(ctx context.Context) (*AccountConfig, *models.WazzupIntegration, error) {
	var lastErr error = ErrAccountNotConnected
	for _, acc := range s.accounts {
		integration, err := s.connection(ctx, acc)
		if err == nil {
			return acc, integration, nil
		}
		if !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrDisabled) {
			return nil, nil, err
		}
		lastErr = err
	}
	return nil, nil, lastErr
}

// accountOfChannel находит аккаунт, которому принадлежит номер (по
// external_channel_id). Возвращает nil без ошибки, если номер не найден ни в
// одном подключённом аккаунте.
func (s *Service) accountOfChannel(ctx context.Context, externalChannelID string) (*AccountConfig, *models.WazzupIntegration, error) {
	externalChannelID = strings.TrimSpace(externalChannelID)
	if externalChannelID == "" {
		return nil, nil, nil
	}
	return s.findChannelAccount(ctx, func(ch models.WazzupChannel) bool {
		return strings.TrimSpace(ch.ExternalChannelID) == externalChannelID
	})
}

// accountOfChannelRow — то же по id строки канала в CRM.
func (s *Service) accountOfChannelRow(ctx context.Context, channelID int64) (*AccountConfig, *models.WazzupIntegration, *models.WazzupChannel, error) {
	var found *models.WazzupChannel
	acc, integration, err := s.findChannelAccount(ctx, func(ch models.WazzupChannel) bool {
		if ch.ID == channelID {
			c := ch
			found = &c
			return true
		}
		return false
	})
	return acc, integration, found, err
}

func (s *Service) findChannelAccount(ctx context.Context, match func(models.WazzupChannel) bool) (*AccountConfig, *models.WazzupIntegration, error) {
	for _, acc := range s.accounts {
		integration, err := s.connection(ctx, acc)
		if err != nil {
			if errors.Is(err, ErrNotFound) || errors.Is(err, ErrDisabled) {
				continue
			}
			return nil, nil, err
		}
		channels, err := s.repo.ListChannels(ctx, integration.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, ch := range channels {
			if match(ch) {
				return acc, integration, nil
			}
		}
	}
	return nil, nil, nil
}

// apiKeyFor — ключ для вызовов аккаунта. Основной берёт ключ из конфига, а
// если его там нет — сохранённый при подключении. Дочернему ключ не нужен.
func (s *Service) apiKeyFor(acc *AccountConfig, integration *models.WazzupIntegration) string {
	if acc.Partner {
		return ""
	}
	if acc.APIKey != "" {
		return acc.APIKey
	}
	if integration != nil {
		return strings.TrimSpace(integration.APIKeyEnc)
	}
	return ""
}

// Accounts — список аккаунтов и их состояние для настроек и меню.
func (s *Service) Accounts(ctx context.Context) ([]AccountInfo, error) {
	out := make([]AccountInfo, 0, len(s.accounts))
	for _, acc := range s.accounts {
		info := AccountInfo{Account: acc.Name, Title: acc.Title, Partner: acc.Partner}
		integration, err := s.repo.GetIntegrationByAccount(ctx, acc.Name)
		if err != nil {
			return nil, err
		}
		if integration != nil {
			info.Connected = true
			info.Enabled = integration.Enabled
			info.WebhookURL = integration.WebhooksURI
		}
		out = append(out, info)
	}
	return out, nil
}

// AdoptLegacy закрепляет за аккаунтом подключение, которое работало до
// появления аккаунтов. Вызывается при старте один раз: после этого текущий
// аккаунт продолжает работать без изменений — те же каналы, филиалы и
// webhook-токен. Возвращает id закреплённого подключения или 0.
func (s *Service) AdoptLegacy(ctx context.Context, account string) (int, error) {
	acc, err := s.accountByName(account)
	if err != nil {
		return 0, err
	}
	return s.repo.AdoptLegacyIntegration(ctx, acc.Name)
}
