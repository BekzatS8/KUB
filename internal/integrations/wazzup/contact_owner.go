package wazzup

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
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
// apiKey нужен основному аккаунту (User API v3), партнёрский его игнорирует.
type contactUpserter interface {
	UpsertContacts(ctx context.Context, apiKey string, contacts []ContactUpsert) error
}

// UpsertContacts создаёт или обновляет контакты (POST /v2/contacts).
func (c *PartnerClient) UpsertContacts(ctx context.Context, _ string, contacts []ContactUpsert) error {
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

// chatContactNamer — репозитории, знающие имя клиента переписки.
type chatContactNamer interface {
	ChatContactName(ctx context.Context, transport, externalChatID string) (string, error)
}

// leadContact — контакт Wazzup переписки: ответственный за чат и ссылка на лид.
//
// Ссылка uri — кнопка в выпадающем списке «Сделки» в шапке чата Wazzup
// (ТЗ «доработки CRM КУБ», п.4; замечание 19.11.2026 «в мессенджере кнопка
// перейти на лид»): страница CRM находит последний лид контакта и открывает
// его карточку.
func (s *Service) leadContact(transport, chatID, name, wazzupUserID string) ContactUpsert {
	chatType := normalizePartnerChatType(transport)
	base := s.crmBaseURL
	if base == "" {
		base = strings.TrimRight(s.webhookBaseURL, "/")
	}
	if strings.TrimSpace(name) == "" {
		name = chatID
	}
	return ContactUpsert{
		ID:                "kub-" + chatType + "-" + chatID,
		ResponsibleUserID: wazzupUserID,
		Name:              name,
		ChatType:          chatType,
		ChatID:            chatID,
		URI:               base + "/leads/by-chat?transport=" + url.QueryEscape(chatType) + "&chat_id=" + url.QueryEscape(chatID),
	}
}

// assignContact делает сотрудника ответственным за чат в Wazzup и заодно
// даёт чату ссылку на лид. Ошибка не прерывает отправку: сообщение всё равно
// уйдёт, а если Wazzup откажет — причина будет в логе статусов.
func (s *Service) assignContact(ctx context.Context, acc *AccountConfig, apiKey, transport, chatID, wazzupUserID string) {
	upserter, ok := acc.Client.(contactUpserter)
	if !ok || chatID == "" || wazzupUserID == "" {
		return
	}
	name := ""
	if namer, ok := s.repo.(chatContactNamer); ok {
		name, _ = namer.ChatContactName(ctx, normalizeTransport(transport), chatID)
	}
	contact := s.leadContact(transport, chatID, name, wazzupUserID)
	if err := upserter.UpsertContacts(ctx, apiKey, []ContactUpsert{contact}); err != nil {
		log.Printf("integration=wazzup operation=assign_contact status=failed chat=%s user=%s err=%v", maskChatID(chatID), wazzupUserID, err)
		return
	}
	log.Printf("integration=wazzup operation=assign_contact status=ok chat=%s user=%s", maskChatID(chatID), wazzupUserID)
}

// AssignChatOwner — ответственный за лид в CRM становится ответственным за
// его переписку в Wazzup. Роль «Менеджер» видит только своих клиентов: без
// этого после «Взять себе» или передачи лида кнопка «Открыть в мессенджере»
// показывала чужой чат вместо переписки с клиентом. Контакт получает и ссылку
// на лид (кнопка в шапке чата). С 19.11.2026 — в обоих аккаунтах (решение
// заказчика: ответственный в Wazzup = ответственный за лид в CRM). Только для
// переписок, которые уже есть в CRM; ошибки лишь пишутся в лог.
func (s *Service) AssignChatOwner(ctx context.Context, transport, chatID string, crmUserID int) {
	transport = normalizeTransport(transport)
	chatID = strings.TrimSpace(chatID)
	if transport == "whatsapp" {
		chatID = digitsOnly(chatID)
	}
	if transport == "" || chatID == "" || crmUserID <= 0 {
		return
	}
	chID, err := s.repo.GetChatChannelID(ctx, transport, chatID)
	if err != nil || chID == "" {
		return
	}
	acc, integration, err := s.accountOfChannel(ctx, chID)
	if err != nil || acc == nil || integration == nil {
		return
	}
	apiKey := s.apiKeyFor(acc, integration)
	wazzupUserID := s.ensureWazzupUser(ctx, acc, apiKey, crmUserID)
	if wazzupUserID == "" {
		return
	}
	s.assignContact(ctx, acc, apiKey, transport, chatID, wazzupUserID)
}

// chatLeadOwnerRepo — репозитории, знающие ответственного за лид переписки.
type chatLeadOwnerRepo interface {
	ChatLeadOwner(ctx context.Context, transport, externalChatID string) (int, error)
}

// assignOwnChatOnOpen — переписку открывает ответственный за её лид в CRM:
// закрепляем чат за ним и в Wazzup, иначе окно откроется на чужом чате.
// Чинит и лиды, взятые до того, как передача начала синхронизироваться.
func (s *Service) assignOwnChatOnOpen(ctx context.Context, acc *AccountConfig, transport, chatID string, crmUserID int) {
	if acc == nil || crmUserID <= 0 {
		return
	}
	repo, ok := s.repo.(chatLeadOwnerRepo)
	if !ok {
		return
	}
	owner, err := repo.ChatLeadOwner(ctx, transport, chatID)
	if err != nil || owner != crmUserID {
		return
	}
	s.AssignChatOwner(ctx, transport, chatID, crmUserID)
}

// chatLeadContactsLister — репозитории, отдающие переписки с ответственным за
// их лид (для начальной выгрузки контактов в Wazzup).
type chatLeadContactsLister interface {
	ListChatLeadContacts(ctx context.Context, account string) ([]repositories.ChatLeadContact, error)
}

// contactsBatch — сколько контактов Wazzup принимает за один запрос.
const contactsBatch = 100

// SyncLeadContacts выгружает в Wazzup контакты всех переписок, у лида которых
// есть ответственный: ответственный за чат и ссылка на лид в шапке чата.
// Запускается при старте сервера; повторный запуск лишь обновляет те же
// контакты. Ошибки пишутся в лог и не мешают работе.
func (s *Service) SyncLeadContacts(ctx context.Context) {
	lister, ok := s.repo.(chatLeadContactsLister)
	if !ok {
		return
	}
	for _, acc := range s.accounts {
		upserter, ok := acc.Client.(contactUpserter)
		if !ok {
			continue
		}
		integration, err := s.repo.GetIntegrationByAccount(ctx, acc.Name)
		if err != nil || integration == nil || !integration.Enabled {
			continue
		}
		apiKey := s.apiKeyFor(acc, integration)
		if !acc.Partner && apiKey == "" {
			continue
		}
		rows, err := lister.ListChatLeadContacts(ctx, acc.Name)
		if err != nil {
			log.Printf("integration=wazzup operation=lead_contacts status=failed account=%s err=%v", acc.Name, err)
			continue
		}
		if len(rows) == 0 {
			continue
		}

		// Ответственные должны быть пользователями Wazzup.
		seen := map[int]bool{}
		users := make([]UserUpsert, 0)
		for _, r := range rows {
			if seen[r.OwnerID] {
				continue
			}
			seen[r.OwnerID] = true
			name := strings.TrimSpace(r.OwnerName)
			if name == "" {
				name = fmt.Sprintf("User %d", r.OwnerID)
			}
			users = append(users, UserUpsert{ID: wazzupUserIDFor(0, r.OwnerID), Name: name})
		}
		if err := acc.Client.UpsertUsers(ctx, apiKey, users); err != nil {
			log.Printf("integration=wazzup operation=lead_contacts status=failed account=%s reason=users err=%v", acc.Name, err)
			continue
		}

		sent, failed := 0, 0
		for start := 0; start < len(rows); start += contactsBatch {
			end := start + contactsBatch
			if end > len(rows) {
				end = len(rows)
			}
			batch := make([]ContactUpsert, 0, end-start)
			for _, r := range rows[start:end] {
				batch = append(batch, s.leadContact(r.Transport, r.ChatID, r.Name, wazzupUserIDFor(0, r.OwnerID)))
			}
			if err := upserter.UpsertContacts(ctx, apiKey, batch); err != nil {
				failed += len(batch)
				log.Printf("integration=wazzup operation=lead_contacts status=failed account=%s batch_from=%d err=%v", acc.Name, start, err)
				continue
			}
			sent += len(batch)
		}
		log.Printf("integration=wazzup operation=lead_contacts status=done account=%s contacts=%d failed=%d owners=%d", acc.Name, sent, failed, len(users))
	}
}
