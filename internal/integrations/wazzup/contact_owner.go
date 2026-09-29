package wazzup

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"turcompany/internal/models"
)

// «Написать первым» от менеджера. В Wazzup роль «Менеджер» (seller) видит и
// пишет только клиентам, за которых она ответственна; клиент, которому раньше
// писал кто-то другой, закреплён за ним, и партнёрский API отвечает
// chat_no_access. Решение заказчика: клиент переходит к тому, кто пишет —
// перед отправкой CRM назначает менеджера ответственным (POST /v2/contacts).
// Руководитель и контроль качества видят всё и так, их это не касается.

// ContactUpsert — контакт Wazzup с ответственным сотрудником.
type ContactUpsert struct {
	ID                string
	ResponsibleUserID string
	Name              string
	ChatType          string
	ChatID            string
	URI               string
}

// contactUpserter — клиенты, умеющие создавать и обновлять контакты.
type contactUpserter interface {
	UpsertContacts(ctx context.Context, contacts []ContactUpsert) error
}

// UpsertContacts создаёт или обновляет контакты (POST /v2/contacts).
func (c *PartnerClient) UpsertContacts(ctx context.Context, contacts []ContactUpsert) error {
	items := make([]map[string]any, 0, len(contacts))
	for _, ct := range contacts {
		items = append(items, map[string]any{
			"id":                  ct.ID,
			"responsible_user_id": ct.ResponsibleUserID,
			"name":                ct.Name,
			"contact_data": []map[string]any{{
				"chat_type": normalizePartnerChatType(ct.ChatType),
				"chat_id":   ct.ChatID,
			}},
			"uri": ct.URI,
		})
	}
	if len(items) == 0 {
		return nil
	}
	_, err := c.doJSON(ctx, http.MethodPost, "/v2/contacts", map[string]any{"contacts": items})
	return err
}

// SetCRMBaseURL — адрес CRM для ссылки из карточки контакта в Wazzup.
func (s *Service) SetCRMBaseURL(u string) {
	s.crmBaseURL = strings.TrimRight(strings.TrimSpace(u), "/")
}

// ErrNoChannelAccess — у сотрудника нет права писать с этого номера.
var ErrNoChannelAccess = fmt.Errorf("%w: нет доступа к номеру", ErrBadRequest)

// senderRole — роль отправителя на номере аккаунта без кабинета: из ручной
// настройки «Доступа» или автоматическая по роли CRM. Без роли и с ролью
// «Контроль качества» писать нельзя — отвечаем сразу, иначе партнёрский API
// примет сообщение и через секунду откажет с chat_no_access.
func (s *Service) senderRole(ctx context.Context, acc *AccountConfig, integration *models.WazzupIntegration, channelID string, userID int) (string, error) {
	if acc == nil || !acc.Partner || channelID == "" || userID <= 0 {
		return "", nil
	}
	channels, err := s.repo.ListChannels(ctx, integration.ID)
	if err != nil {
		return "", nil // проверка вспомогательная — не мешаем отправке
	}
	for _, ch := range channels {
		if strings.TrimSpace(ch.ExternalChannelID) != channelID {
			continue
		}
		if !ch.RolesConfigured {
			break
		}
		roles, err := s.repo.ListChannelRoles(ctx, ch.ID)
		if err != nil {
			return "", nil
		}
		name := ch.Name
		if name == "" {
			name = ch.Phone
		}
		for _, r := range roles {
			if r.UserID != userID {
				continue
			}
			if r.Role == "auditor" {
				return "", fmt.Errorf("%w %s: у вас роль «Контроль качества» — писать с этого номера нельзя", ErrNoChannelAccess, name)
			}
			return r.Role, nil
		}
		return "", fmt.Errorf("%w %s: попросите администратора выдать вам роль в «Каналы мессенджера» → «Доступ»", ErrNoChannelAccess, name)
	}
	// Автоматические роли — по роли в CRM.
	users, err := s.repo.ListCRMUsers(ctx)
	if err != nil {
		return "", nil
	}
	for _, u := range users {
		if u.ID == userID {
			return wazzupRoleFor(u.RoleID), nil
		}
	}
	return "", nil
}

// assignContact делает менеджера ответственным за чат перед «Написать первым».
// Ошибка не прерывает отправку: сообщение всё равно уйдёт, а если Wazzup
// откажет — причина будет в логе статусов (operation=message_status).
func (s *Service) assignContact(ctx context.Context, acc *AccountConfig, transport, chatID, wazzupUserID string) {
	upserter, ok := acc.Client.(contactUpserter)
	if !ok || chatID == "" || wazzupUserID == "" {
		return
	}
	chatType := normalizePartnerChatType(transport)
	base := s.crmBaseURL
	if base == "" {
		base = strings.TrimRight(s.webhookBaseURL, "/")
	}
	contact := ContactUpsert{
		ID:                "kub-" + chatType + "-" + chatID,
		ResponsibleUserID: wazzupUserID,
		Name:              chatID,
		ChatType:          chatType,
		ChatID:            chatID,
		URI:               base + "/whatsapp?transport=" + url.QueryEscape(chatType) + "&chat_id=" + url.QueryEscape(chatID),
	}
	if err := upserter.UpsertContacts(ctx, []ContactUpsert{contact}); err != nil {
		log.Printf("integration=wazzup operation=assign_contact status=failed chat=%s user=%s err=%v", maskChatID(chatID), wazzupUserID, err)
		return
	}
	log.Printf("integration=wazzup operation=assign_contact status=ok chat=%s user=%s", maskChatID(chatID), wazzupUserID)
}
