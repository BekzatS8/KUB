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

// chatLeadFinder — репозитории, знающие последний лид переписки.
type chatLeadFinder interface {
	ChatLead(ctx context.Context, transport, externalChatID string) (*repositories.ChatLeadContact, error)
}

// DealUpsert — сделка Wazzup: лид CRM в списке «Сделки» в шапке чата.
type DealUpsert struct {
	ID                string
	ResponsibleUserID string
	Name              string
	URI               string
	ContactIDs        []string
	Closed            bool
}

// dealUpserter — клиенты, умеющие выгружать сделки (основной аккаунт).
type dealUpserter interface {
	UpsertDeals(ctx context.Context, apiKey string, deals []DealUpsert) error
}

// contactResponsibleReader — клиенты, знающие ответственных контактов Wazzup
// (основной аккаунт): лид без ответственного в CRM не должен снимать
// назначение, сделанное в самом Wazzup или при отправке из CRM.
type contactResponsibleReader interface {
	ContactResponsibles(ctx context.Context, apiKey string) (map[string]string, error)
	ContactResponsible(ctx context.Context, apiKey, contactID string) (string, error)
}

// chatLeadChatsRepo — репозитории, знающие все переписки лида.
type chatLeadChatsRepo interface {
	LeadChats(ctx context.Context, leadID int, account string) ([]repositories.ChatLeadContact, error)
}

// mergedLeadsLister — репозитории, отдающие объединённые лиды.
type mergedLeadsLister interface {
	ListMergedLeads(ctx context.Context) ([]repositories.MergedLead, error)
}

func (s *Service) crmBase() string {
	if s.crmBaseURL != "" {
		return s.crmBaseURL
	}
	return strings.TrimRight(s.webhookBaseURL, "/")
}

// leadDeal — лид как сделка Wazzup (замечание 19.11.2026: в шапке чата было
// «Нет открытых сделок»). Ссылка ведёт прямо на карточку лида; закрыта —
// лид конвертирован или отменён. contactIDs — все переписки лида: после
// объединения лида из Instagram с лидом из WhatsApp сделка видна в обоих чатах.
func (s *Service) leadDeal(lead repositories.ChatLeadContact, contactIDs []string, wazzupUserID string) DealUpsert {
	base := s.crmBase()
	name := fmt.Sprintf("Лид #%d", lead.LeadID)
	if t := strings.TrimSpace(lead.LeadTitle); t != "" {
		name += " — " + t
	}
	if r := []rune(name); len(r) > 200 {
		name = string(r[:200])
	}
	return DealUpsert{
		ID:                fmt.Sprintf("%d", lead.LeadID),
		ResponsibleUserID: wazzupUserID,
		Name:              name,
		URI:               fmt.Sprintf("%s/leads/%d", base, lead.LeadID),
		ContactIDs:        contactIDs,
		Closed:            lead.LeadClosed,
	}
}

// mergedDeal — сделка объединённого лида (замечание 20.11.2026: из чата
// Instagram после объединения ссылка вела на удалённый лид, «Лид не найден»).
// Сделку не удаляем, а закрываем и ведём на основной лид.
func (s *Service) mergedDeal(sourceID, targetID int, contactIDs []string, wazzupUserID string) DealUpsert {
	return DealUpsert{
		ID:                fmt.Sprintf("%d", sourceID),
		ResponsibleUserID: wazzupUserID,
		Name:              fmt.Sprintf("Лид #%d — объединён с лидом #%d", sourceID, targetID),
		URI:               fmt.Sprintf("%s/leads/%d", s.crmBase(), targetID),
		ContactIDs:        contactIDs,
		Closed:            true,
	}
}

// leadContact — контакт Wazzup переписки: ответственный за чат и ссылка на лид.
//
// Ссылка uri — кнопка в выпадающем списке «Сделки» в шапке чата Wazzup
// (ТЗ «доработки CRM КУБ», п.4; замечание 19.11.2026 «в мессенджере кнопка
// перейти на лид»): страница CRM находит последний лид контакта и открывает
// его карточку.
func (s *Service) leadContact(transport, chatID, name, wazzupUserID string) ContactUpsert {
	chatType := normalizePartnerChatType(transport)
	base := s.crmBase()
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
	var lead *repositories.ChatLeadContact
	if finder, ok := s.repo.(chatLeadFinder); ok {
		lead, _ = finder.ChatLead(ctx, normalizeTransport(transport), chatID)
	}
	name := ""
	if lead != nil {
		name = lead.Name
	}
	contact := s.leadContact(transport, chatID, name, wazzupUserID)
	contacts := []ContactUpsert{contact}
	// Остальные переписки того же лида (объединённые лиды) — тому же
	// сотруднику: сделка лида ссылается на все его чаты.
	if lister, ok := s.repo.(chatLeadChatsRepo); ok && lead != nil && lead.LeadID > 0 {
		if others, err := lister.LeadChats(ctx, lead.LeadID, acc.Name); err == nil {
			for _, o := range others {
				c := s.leadContact(o.Transport, o.ChatID, o.Name, wazzupUserID)
				if c.ID != contact.ID {
					contacts = append(contacts, c)
				}
			}
		}
	}
	if err := upserter.UpsertContacts(ctx, apiKey, contacts); err != nil {
		log.Printf("integration=wazzup operation=assign_contact status=failed chat=%s user=%s err=%v", maskChatID(chatID), wazzupUserID, err)
		return
	}
	log.Printf("integration=wazzup operation=assign_contact status=ok chat=%s user=%s contacts=%d", maskChatID(chatID), wazzupUserID, len(contacts))
	if dealer, ok := acc.Client.(dealUpserter); ok && lead != nil && lead.LeadID > 0 {
		ids := make([]string, 0, len(contacts))
		for _, c := range contacts {
			ids = append(ids, c.ID)
		}
		if err := dealer.UpsertDeals(ctx, apiKey, []DealUpsert{s.leadDeal(*lead, ids, wazzupUserID)}); err != nil {
			log.Printf("integration=wazzup operation=lead_deal status=failed lead_id=%d err=%v", lead.LeadID, err)
		}
	}
}

// LeadsMerged — после «Объединить лиды»: переписки обоих лидов теперь у
// основного лида. Его ответственный получает все чаты, сделка основного лида
// видна во всех чатах, а сделка влитого лида закрывается и ведёт на основной.
func (s *Service) LeadsMerged(ctx context.Context, targetID, sourceID, crmOwnerID int) {
	lister, ok := s.repo.(chatLeadChatsRepo)
	if !ok || targetID <= 0 || sourceID <= 0 || crmOwnerID <= 0 {
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
		chats, err := lister.LeadChats(ctx, targetID, acc.Name)
		if err != nil || len(chats) == 0 {
			continue
		}
		wazzupUserID := s.ensureWazzupUser(ctx, acc, apiKey, crmOwnerID)
		if wazzupUserID == "" {
			continue
		}
		contacts := make([]ContactUpsert, 0, len(chats))
		ids := make([]string, 0, len(chats))
		for _, c := range chats {
			ct := s.leadContact(c.Transport, c.ChatID, c.Name, wazzupUserID)
			contacts = append(contacts, ct)
			ids = append(ids, ct.ID)
		}
		if err := upserter.UpsertContacts(ctx, apiKey, contacts); err != nil {
			log.Printf("integration=wazzup operation=lead_merge status=failed account=%s lead_id=%d err=%v", acc.Name, targetID, err)
			continue
		}
		dealer, ok := acc.Client.(dealUpserter)
		if !ok {
			continue
		}
		deals := []DealUpsert{s.leadDeal(chats[0], ids, wazzupUserID), s.mergedDeal(sourceID, targetID, ids, wazzupUserID)}
		if err := dealer.UpsertDeals(ctx, apiKey, deals); err != nil {
			log.Printf("integration=wazzup operation=lead_merge status=failed account=%s lead_id=%d reason=deals err=%v", acc.Name, targetID, err)
			continue
		}
		log.Printf("integration=wazzup operation=lead_merge status=ok account=%s lead_id=%d source_id=%d contacts=%d", acc.Name, targetID, sourceID, len(contacts))
	}
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

// SyncLeadContacts выгружает в Wazzup контакты и сделки всех переписок с
// лидом: ответственный за чат и ссылка на лид в шапке чата.
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

		// Лиды без ответственного: сделка нужна и им (переход к лиду из
		// чата), но ответственного в Wazzup не трогаем — оставляем того, кто
		// уже назначен, иначе никого. Партнёрский аккаунт сделок не знает.
		reader, canRead := acc.Client.(contactResponsibleReader)
		var current map[string]string
		if !acc.Partner && canRead {
			for _, r := range rows {
				if r.OwnerID == 0 {
					if current, err = reader.ContactResponsibles(ctx, apiKey); err != nil {
						log.Printf("integration=wazzup operation=lead_contacts status=failed account=%s reason=contacts_list err=%v", acc.Name, err)
						current = nil
					}
					break
				}
			}
		}
		kept := rows[:0:0]
		for _, r := range rows {
			if r.OwnerID == 0 && current == nil {
				continue
			}
			kept = append(kept, r)
		}
		rows = kept
		if len(rows) == 0 {
			continue
		}
		responsibleOf := func(r repositories.ChatLeadContact) string {
			if r.OwnerID > 0 {
				return wazzupUserIDFor(0, r.OwnerID)
			}
			return current["kub-"+normalizePartnerChatType(r.Transport)+"-"+r.ChatID]
		}

		// Ответственные должны быть пользователями Wazzup.
		seen := map[int]bool{0: true}
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
		if len(users) > 0 {
			if err := acc.Client.UpsertUsers(ctx, apiKey, users); err != nil {
				log.Printf("integration=wazzup operation=lead_contacts status=failed account=%s reason=users err=%v", acc.Name, err)
				continue
			}
		}

		// Сначала все контакты, затем сделки: сделка ссылается на все
		// переписки своего лида, а они могут попасть в разные пачки.
		sent, failed, dealsSent := 0, 0, 0
		leadContacts := map[int][]string{}
		leadRow := map[int]repositories.ChatLeadContact{}
		leadOrder := make([]int, 0)
		for start := 0; start < len(rows); start += contactsBatch {
			end := start + contactsBatch
			if end > len(rows) {
				end = len(rows)
			}
			batch := make([]ContactUpsert, 0, end-start)
			for _, r := range rows[start:end] {
				batch = append(batch, s.leadContact(r.Transport, r.ChatID, r.Name, responsibleOf(r)))
			}
			if err := upserter.UpsertContacts(ctx, apiKey, batch); err != nil {
				failed += len(batch)
				log.Printf("integration=wazzup operation=lead_contacts status=failed account=%s batch_from=%d err=%v", acc.Name, start, err)
				continue
			}
			sent += len(batch)
			for i, r := range rows[start:end] {
				if r.LeadID <= 0 {
					continue
				}
				if _, seen := leadRow[r.LeadID]; !seen {
					leadRow[r.LeadID] = r
					leadOrder = append(leadOrder, r.LeadID)
				}
				leadContacts[r.LeadID] = append(leadContacts[r.LeadID], batch[i].ID)
			}
		}
		if dealer, ok := acc.Client.(dealUpserter); ok && len(leadOrder) > 0 {
			deals := make([]DealUpsert, 0, len(leadOrder))
			for _, id := range leadOrder {
				r := leadRow[id]
				deals = append(deals, s.leadDeal(r, leadContacts[id], responsibleOf(r)))
			}
			// Сделки объединённых лидов ведут на основной лид.
			if ml, ok := s.repo.(mergedLeadsLister); ok {
				if merged, err := ml.ListMergedLeads(ctx); err == nil {
					for _, m := range merged {
						if r, ok := leadRow[m.TargetID]; ok {
							deals = append(deals, s.mergedDeal(m.SourceID, m.TargetID, leadContacts[m.TargetID], responsibleOf(r)))
						}
					}
				}
			}
			for start := 0; start < len(deals); start += contactsBatch {
				end := start + contactsBatch
				if end > len(deals) {
					end = len(deals)
				}
				if err := dealer.UpsertDeals(ctx, apiKey, deals[start:end]); err != nil {
					log.Printf("integration=wazzup operation=lead_deals status=failed account=%s batch_from=%d err=%v", acc.Name, start, err)
					continue
				}
				dealsSent += end - start
			}
		}
		log.Printf("integration=wazzup operation=lead_contacts status=done account=%s contacts=%d deals=%d failed=%d owners=%d", acc.Name, sent, dealsSent, failed, len(users))
	}
}

// LeadCreatedFromChat — входящее сообщение создало лид (без ответственного):
// сразу даём чату сделку со ссылкой на лид, чтобы из Wazzup можно было
// перейти к карточке (замечание 20.11.2026). Ответственного в Wazzup не
// меняем. Только основной аккаунт; ошибки — в лог.
func (s *Service) LeadCreatedFromChat(ctx context.Context, transport, chatID string) {
	transport = normalizeTransport(transport)
	chatID = strings.TrimSpace(chatID)
	if transport == "whatsapp" {
		chatID = digitsOnly(chatID)
	}
	if transport == "" || chatID == "" {
		return
	}
	finder, ok := s.repo.(chatLeadFinder)
	if !ok {
		return
	}
	chID, err := s.repo.GetChatChannelID(ctx, transport, chatID)
	if err != nil || chID == "" {
		return
	}
	acc, integration, err := s.accountOfChannel(ctx, chID)
	if err != nil || acc == nil || integration == nil || acc.Partner {
		return
	}
	upserter, ok1 := acc.Client.(contactUpserter)
	dealer, ok2 := acc.Client.(dealUpserter)
	reader, ok3 := acc.Client.(contactResponsibleReader)
	apiKey := s.apiKeyFor(acc, integration)
	if !ok1 || !ok2 || !ok3 || apiKey == "" {
		return
	}
	lead, err := finder.ChatLead(ctx, transport, chatID)
	if err != nil || lead == nil || lead.LeadID <= 0 {
		return
	}
	responsible := ""
	if lead.OwnerID > 0 {
		responsible = s.ensureWazzupUser(ctx, acc, apiKey, lead.OwnerID)
	} else {
		contactID := "kub-" + normalizePartnerChatType(transport) + "-" + chatID
		if responsible, err = reader.ContactResponsible(ctx, apiKey, contactID); err != nil {
			log.Printf("integration=wazzup operation=new_lead_deal status=failed lead_id=%d reason=contact err=%v", lead.LeadID, err)
			return
		}
	}
	contact := s.leadContact(transport, chatID, lead.Name, responsible)
	if err := upserter.UpsertContacts(ctx, apiKey, []ContactUpsert{contact}); err != nil {
		log.Printf("integration=wazzup operation=new_lead_deal status=failed lead_id=%d err=%v", lead.LeadID, err)
		return
	}
	if err := dealer.UpsertDeals(ctx, apiKey, []DealUpsert{s.leadDeal(*lead, []string{contact.ID}, responsible)}); err != nil {
		log.Printf("integration=wazzup operation=new_lead_deal status=failed lead_id=%d reason=deal err=%v", lead.LeadID, err)
		return
	}
	log.Printf("integration=wazzup operation=new_lead_deal status=ok lead_id=%d chat=%s", lead.LeadID, maskChatID(chatID))
}
