package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

// CalendarService — встречи с клиентами (ТЗ «доработки CRM КУБ», п.6).
// Руководство, админ и ОКК видят все встречи (с фильтром по филиалу и
// сотруднику); остальные — свои (ответственный или назначил сам) и встречи
// по своим лидам. Менять и удалять — ответственный, автор, руководство, админ.
type CalendarService struct {
	repo  *repositories.CalendarRepository
	leads *LeadService
}

func NewCalendarService(repo *repositories.CalendarRepository, leads *LeadService) *CalendarService {
	return &CalendarService{repo: repo, leads: leads}
}

// CalendarValidationError — неверные данные встречи; Msg — для пользователя.
type CalendarValidationError struct{ Msg string }

func (e *CalendarValidationError) Error() string { return e.Msg }

func calendarSeesAll(roleID int) bool {
	return authz.CanViewAllBusinessData(roleID)
}

func canEditCalendarEvent(e *models.CalendarEvent, userID, roleID int) bool {
	if e == nil || authz.IsReadOnly(roleID) {
		return false
	}
	if authz.IsFullAccess(roleID) {
		return true
	}
	return (e.OwnerID != nil && *e.OwnerID == userID) || (e.CreatedBy != nil && *e.CreatedBy == userID)
}

func (s *CalendarService) List(ctx context.Context, f models.CalendarEventFilter, userID, roleID int) ([]*models.CalendarEvent, error) {
	if !authz.IsKnownRole(roleID) {
		return nil, ErrForbidden
	}
	if f.To.Before(f.From) || f.To.Sub(f.From) > 120*24*time.Hour {
		return nil, &CalendarValidationError{Msg: "Период — не больше 120 дней"}
	}
	if !calendarSeesAll(roleID) {
		uid := userID
		f.VisibleTo = &uid
		f.BranchID = nil
	}
	items, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, err
	}
	for _, e := range items {
		e.CanEdit = canEditCalendarEvent(e, userID, roleID)
	}
	return items, nil
}

// normalize проверяет поля встречи и проставляет ответственного/филиал.
// keepLeadID — лид, уже привязанный к встрече: его доступность не
// перепроверяем (оператор назначил встречу, лид потом забрал МОП — правка
// встречи не должна ломаться).
func (s *CalendarService) normalize(ctx context.Context, e *models.CalendarEvent, userID, roleID int, keepLeadID *int) error {
	e.Title = strings.TrimSpace(e.Title)
	e.Description = strings.TrimSpace(e.Description)
	if e.Title == "" {
		return &CalendarValidationError{Msg: "Укажите название встречи"}
	}
	if e.StartsAt.IsZero() {
		return &CalendarValidationError{Msg: "Укажите дату и время встречи"}
	}
	if e.EndsAt != nil && e.EndsAt.Before(e.StartsAt) {
		return &CalendarValidationError{Msg: "Окончание раньше начала"}
	}
	// Ответственного выбирают руководство и админ; остальные ставят встречу себе.
	if e.OwnerID == nil || *e.OwnerID <= 0 || !authz.IsFullAccess(roleID) {
		uid := userID
		e.OwnerID = &uid
	}
	sameLead := e.LeadID != nil && keepLeadID != nil && *e.LeadID == *keepLeadID
	if e.LeadID != nil && *e.LeadID > 0 && s.leads != nil && !sameLead {
		lead, err := s.leads.GetByID(*e.LeadID, userID, roleID)
		if err != nil || lead == nil {
			return &CalendarValidationError{Msg: "Лид не найден или недоступен"}
		}
	} else if !sameLead {
		e.LeadID = nil
	}
	if e.ClientID != nil && *e.ClientID <= 0 {
		e.ClientID = nil
	}
	branch, err := s.repo.UserBranch(ctx, *e.OwnerID)
	if err != nil {
		return err
	}
	e.BranchID = branch
	return nil
}

func (s *CalendarService) Create(ctx context.Context, e *models.CalendarEvent, userID, roleID int) (*models.CalendarEvent, error) {
	if !authz.IsKnownRole(roleID) || authz.IsReadOnly(roleID) {
		return nil, ErrForbidden
	}
	if err := s.normalize(ctx, e, userID, roleID, nil); err != nil {
		return nil, err
	}
	uid := userID
	e.CreatedBy = &uid
	id, err := s.repo.Create(ctx, e)
	if err != nil {
		return nil, err
	}
	created, err := s.repo.GetByID(ctx, id)
	if err == nil && created != nil {
		created.CanEdit = true
	}
	return created, err
}

func (s *CalendarService) Update(ctx context.Context, e *models.CalendarEvent, userID, roleID int) (*models.CalendarEvent, error) {
	current, err := s.repo.GetByID(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrNotFound
	}
	if !canEditCalendarEvent(current, userID, roleID) {
		return nil, ErrForbidden
	}
	if err := s.normalize(ctx, e, userID, roleID, current.LeadID); err != nil {
		return nil, err
	}
	// Рядовой сотрудник не перехватывает чужую встречу правкой.
	if !authz.IsFullAccess(roleID) {
		e.OwnerID = current.OwnerID
	}
	if err := s.repo.Update(ctx, e); err != nil {
		return nil, err
	}
	updated, err := s.repo.GetByID(ctx, e.ID)
	if err == nil && updated != nil {
		updated.CanEdit = true
	}
	return updated, err
}

func (s *CalendarService) Delete(ctx context.Context, id int64, userID, roleID int) error {
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrNotFound
	}
	if !canEditCalendarEvent(current, userID, roleID) {
		return ErrForbidden
	}
	return s.repo.Delete(ctx, id)
}

// IsCalendarValidation — ошибка валидации встречи.
func IsCalendarValidation(err error) (string, bool) {
	var v *CalendarValidationError
	if errors.As(err, &v) {
		return v.Msg, true
	}
	return "", false
}
