package services

import (
	"context"
	"errors"
	"log"
	"strings"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

// Карточка лида (ТЗ «доработки CRM КУБ», п.3): «Краткая информация» с
// историей изменений, передача в отдел продаж оператором, «Вернуть лид» в
// общую очередь с причиной и объединение лидов разных каналов.

// LeadBriefRequiredError — для передачи в продажи не заполнены обязательные
// поля краткой информации (ФИО, страна, тип визы).
type LeadBriefRequiredError struct{ Missing []string }

func (e *LeadBriefRequiredError) Error() string {
	return "lead brief incomplete: " + strings.Join(e.Missing, ", ")
}

// ErrLeadReasonRequired — «Вернуть лид» без причины.
var ErrLeadReasonRequired = errors.New("reason is required")

// ErrLeadAlreadyHanded — лид уже передан в отдел продаж.
var ErrLeadAlreadyHanded = errors.New("lead already handed to sales")

var leadBriefFieldLabels = []struct{ key, label string }{
	{"brief_full_name", "ФИО клиента"},
	{"brief_country", "Страна"},
	{"brief_visa_type", "Тип визы"},
	{"brief_city", "Город"},
	{"brief_age", "Возраст"},
	{"brief_comment", "Комментарий"},
}

func briefValue(b models.LeadBrief, key string) string {
	switch key {
	case "brief_full_name":
		return b.FullName
	case "brief_country":
		return b.Country
	case "brief_visa_type":
		return b.VisaType
	case "brief_city":
		return b.City
	case "brief_age":
		return b.Age
	case "brief_comment":
		return b.Comment
	}
	return ""
}

func briefOf(lead *models.Leads) models.LeadBrief {
	return models.LeadBrief{
		FullName: lead.BriefFullName,
		Country:  lead.BriefCountry,
		VisaType: lead.BriefVisaType,
		City:     lead.BriefCity,
		Age:      lead.BriefAge,
		Comment:  lead.BriefComment,
	}
}

func trimBrief(b models.LeadBrief) models.LeadBrief {
	return models.LeadBrief{
		FullName: strings.TrimSpace(b.FullName),
		Country:  strings.TrimSpace(b.Country),
		VisaType: strings.TrimSpace(b.VisaType),
		City:     strings.TrimSpace(b.City),
		Age:      strings.TrimSpace(b.Age),
		Comment:  strings.TrimSpace(b.Comment),
	}
}

// briefChanges — записи истории по изменённым полям (кто и когда изменил).
func briefChanges(before, after models.LeadBrief) []models.LeadHistoryEntry {
	out := []models.LeadHistoryEntry{}
	for _, f := range leadBriefFieldLabels {
		oldV, newV := briefValue(before, f.key), briefValue(after, f.key)
		if oldV != newV {
			out = append(out, models.LeadHistoryEntry{Action: "brief_update", Field: f.key, OldValue: oldV, NewValue: newV})
		}
	}
	return out
}

// MissingForHandoff — какие обязательные для передачи поля пусты.
func MissingForHandoff(b models.LeadBrief) []string {
	missing := []string{}
	if strings.TrimSpace(b.FullName) == "" {
		missing = append(missing, "ФИО клиента")
	}
	if strings.TrimSpace(b.Country) == "" {
		missing = append(missing, "Страна")
	}
	if strings.TrimSpace(b.VisaType) == "" {
		missing = append(missing, "Тип визы")
	}
	return missing
}

// loadLeadForWrite — лид в зоне видимости пользователя и доступный ему на
// изменение (не только на просмотр).
func (s *LeadService) loadLeadForWrite(id, userID, roleID int) (*models.Leads, error) {
	if authz.IsReadOnly(roleID) {
		return nil, ErrReadOnly
	}
	if !authz.CanWorkWithLeads(roleID) {
		return nil, ErrForbidden
	}
	lead, err := s.Repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if lead == nil {
		return nil, ErrLeadNotFound
	}
	scope, err := resolveLeadScope(userID, roleID, s.UserRepo)
	if err != nil {
		return nil, err
	}
	if !leadMatchesScope(scope, lead) {
		return nil, ErrForbidden
	}
	if leadReadOnlyFor(lead, roleID) {
		return nil, ErrReadOnly
	}
	return lead, nil
}

// UpdateBrief сохраняет «Краткую информацию». Заполняет оператор, МОП и
// руководство дополняют; каждое изменение поля пишется в историю.
func (s *LeadService) UpdateBrief(ctx context.Context, id int, brief models.LeadBrief, userID, roleID int) (*models.Leads, error) {
	lead, err := s.loadLeadForWrite(id, userID, roleID)
	if err != nil {
		return nil, err
	}
	brief = trimBrief(brief)
	changes := briefChanges(briefOf(lead), brief)
	if len(changes) > 0 {
		if err := s.Repo.UpdateBrief(ctx, id, userID, brief, changes); err != nil {
			return nil, err
		}
	}
	return s.GetByID(id, userID, roleID)
}

// HandToSales — «Передать в отдел продаж» (оператор; руководство и админ тоже
// могут). Нужны ФИО, страна и тип визы. Лид встаёт на первый этап воронки
// продаж без ответственного; оператору после передачи — только просмотр.
func (s *LeadService) HandToSales(ctx context.Context, id, userID, roleID int) error {
	if !authz.IsOperator(roleID) && !authz.CanMoveLeadBetweenDepartments(roleID) {
		return ErrForbidden
	}
	lead, err := s.loadLeadForWrite(id, userID, roleID)
	if err != nil {
		return err
	}
	if lead.HandedToSalesAt != nil {
		return ErrLeadAlreadyHanded
	}
	if missing := MissingForHandoff(briefOf(lead)); len(missing) > 0 {
		return &LeadBriefRequiredError{Missing: missing}
	}
	funnelID, err := s.Repo.HandToSales(ctx, id, userID)
	if err != nil {
		return err
	}
	log.Printf("[lead][hand_to_sales] lead=%d by user=%d funnel=%d", id, userID, funnelID)
	if s.BoardNotifier != nil {
		if lead.FunnelID != nil && *lead.FunnelID != funnelID {
			s.BoardNotifier.NotifyBoardChanged(*lead.FunnelID)
		}
		s.BoardNotifier.NotifyBoardChanged(funnelID)
	}
	return nil
}

// ReturnLead — «Вернуть лид»: снимает ответственного, лид уходит в общую
// очередь нераспределённых. Причина обязательна и пишется в историю. Вернуть
// может сам ответственный, руководство и админ.
func (s *LeadService) ReturnLead(ctx context.Context, id int, reason string, userID, roleID int) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrLeadReasonRequired
	}
	lead, err := s.loadLeadForWrite(id, userID, roleID)
	if err != nil {
		return err
	}
	if lead.OwnerID == 0 {
		return nil // уже в общей очереди
	}
	if lead.OwnerID != userID && !authz.IsFullAccess(roleID) {
		return ErrForbidden
	}
	ownerName := ""
	if s.UserRepo != nil {
		if u, err := s.UserRepo.GetByID(lead.OwnerID); err == nil && u != nil {
			ownerName = userDisplayName(u)
		}
	}
	if err := s.Repo.ReturnToQueue(ctx, id, userID, lead.OwnerID, ownerName, reason); err != nil {
		return err
	}
	log.Printf("[lead][return] lead=%d owner %d -> queue by user=%d", id, lead.OwnerID, userID)
	s.notifyLeadBoard(lead)
	return nil
}

// MergeLeads — «Объединить лиды»: лид из Instagram с лидом из WhatsApp/
// Telegram и наоборот. Остаётся открытый лид (target), второй (source)
// вливается в него и уходит в корзину. Оба лида должны быть доступны
// пользователю на изменение.
func (s *LeadService) MergeLeads(ctx context.Context, targetID, sourceID, userID, roleID int) error {
	if targetID == sourceID {
		return repositories.ErrLeadMergeConflict
	}
	target, err := s.loadLeadForWrite(targetID, userID, roleID)
	if err != nil {
		return err
	}
	source, err := s.loadLeadForWrite(sourceID, userID, roleID)
	if err != nil {
		return err
	}
	// Рядовой сотрудник не вливает в свой лид чужой рабочий лид коллеги.
	if !authz.IsFullAccess(roleID) {
		for _, l := range []*models.Leads{target, source} {
			if l.OwnerID != 0 && l.OwnerID != userID && !s.ownerIsFree(l.OwnerID) {
				return ErrLeadTaken
			}
		}
	}
	if err := s.Repo.MergeLeads(ctx, targetID, sourceID, userID); err != nil {
		return err
	}
	log.Printf("[lead][merge] lead=%d <- lead=%d by user=%d", targetID, sourceID, userID)
	s.notifyLeadBoard(target)
	if source.FunnelID != nil && (target.FunnelID == nil || *target.FunnelID != *source.FunnelID) {
		s.notifyLeadBoard(source)
	}
	return nil
}

// History — история лида (видна всем, кому виден лид).
func (s *LeadService) History(ctx context.Context, id, userID, roleID int) ([]models.LeadHistoryEntry, error) {
	lead, err := s.GetByIDWithArchiveScope(id, userID, roleID, repositories.ArchiveScopeAll)
	if err != nil {
		return nil, err
	}
	if lead == nil {
		return nil, ErrLeadNotFound
	}
	return s.Repo.ListHistory(ctx, id)
}
