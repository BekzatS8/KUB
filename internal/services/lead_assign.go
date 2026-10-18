package services

import (
	"errors"
	"log"
	"sort"
	"strings"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

// Ответственный за лид: передача (админ/руководство), «Взять себе» (любой
// сотрудник филиала, пока лид ничей) и освобождение лидов уволенного
// сотрудника.

// ErrLeadTaken — лид уже в работе у другого сотрудника.
var ErrLeadTaken = errors.New("lead already has a responsible")

// LeadAssignError — назначить этого сотрудника нельзя; Msg — причина для
// администратора.
type LeadAssignError struct{ Msg string }

func (e *LeadAssignError) Error() string { return "lead assign: " + e.Msg }

// LeadAssignee — сотрудник, которому можно передать лид.
type LeadAssignee struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	RoleID     int    `json:"role_id"`
	BranchID   *int   `json:"branch_id,omitempty"`
	IsElevated bool   `json:"is_elevated"`
}

func userDisplayName(u *models.User) string {
	name := strings.TrimSpace(strings.Join([]string{u.LastName, u.FirstName}, " "))
	if name == "" {
		name = strings.TrimSpace(u.CompanyName)
	}
	if name == "" {
		name = u.Email
	}
	return name
}

// ownerIsFree — лид без рабочего ответственного: владельца нет, он
// заблокирован или не подтверждён, или лид «припаркован» на админе/руководстве
// (так приходят лиды из Instagram/WhatsApp/звонков). Такой лид берёт себе
// любой сотрудник филиала. Без UserRepo — не рискуем: лид занят.
func (s *LeadService) ownerIsFree(ownerID int) bool {
	return ownerIsFreeFor(s.UserRepo, ownerID)
}

// ownerIsFreeFor — общее правило «ничей» для лидов и сделок.
func ownerIsFreeFor(users repositories.UserRepository, ownerID int) bool {
	if ownerID == 0 {
		return true
	}
	if users == nil {
		return false
	}
	if active, verified, err := users.GetAccountStatus(ownerID); err == nil && (!active || !verified) {
		return true
	}
	owner, err := users.GetByID(ownerID)
	if err != nil || owner == nil {
		return false
	}
	return authz.IsElevated(owner.RoleID)
}

// claimsOwnershipOnMove reports whether moving the card also hands the lead to
// the actor. A manager taking a card off the board becomes responsible for it
// when nobody is really working it yet (see ownerIsFree). A lead already owned
// by another working manager keeps its owner: dragging a colleague's card must
// not silently take it from them. Admin/management move cards without taking
// them over.
func (s *LeadService) claimsOwnershipOnMove(lead *models.Leads, userID, roleID int) bool {
	if authz.IsElevated(roleID) {
		return false
	}
	if lead.OwnerID == userID {
		return false
	}
	return s.ownerIsFree(lead.OwnerID)
}

// decorateOwner заполняет имя ответственного и признак «ничей» для карточки.
func (s *LeadService) decorateOwner(lead *models.Leads) {
	if lead == nil {
		return
	}
	free := s.ownerIsFree(lead.OwnerID)
	lead.Unassigned = &free
	if lead.OwnerID != 0 && s.UserRepo != nil {
		if owner, err := s.UserRepo.GetByID(lead.OwnerID); err == nil && owner != nil {
			lead.OwnerName = userDisplayName(owner)
		}
	}
}

// checkAssignee — можно ли передать лид этому сотруднику: он работает в
// системе, работает с лидами и видит лид (тот же филиал; руководство и админ
// видят все филиалы).
func (s *LeadService) checkAssignee(lead *models.Leads, assigneeID int) (*models.User, error) {
	if s.UserRepo == nil {
		return nil, &LeadAssignError{Msg: "Справочник сотрудников недоступен"}
	}
	assignee, err := s.UserRepo.GetByID(assigneeID)
	if err != nil || assignee == nil {
		return nil, &LeadAssignError{Msg: "Сотрудник не найден"}
	}
	active, verified, err := s.UserRepo.GetAccountStatus(assigneeID)
	if err != nil {
		return nil, err
	}
	if !active || !verified {
		return nil, &LeadAssignError{Msg: "Сотрудник заблокирован или не подтверждён"}
	}
	if !authz.CanWorkWithLeads(assignee.RoleID) {
		return nil, &LeadAssignError{Msg: "Этот сотрудник не работает с лидами"}
	}
	if !authz.IsElevated(assignee.RoleID) && lead.BranchID != nil &&
		(assignee.BranchID == nil || *assignee.BranchID != *lead.BranchID) {
		return nil, &LeadAssignError{Msg: "Сотрудник из другого филиала — лид ему не будет виден"}
	}
	return assignee, nil
}

// AssignOwner передаёт лид сотруднику (админ и руководство).
func (s *LeadService) AssignOwner(id, assigneeID, userID, roleID int) error {
	if roleID != authz.RoleManagement && roleID != authz.RoleSystemAdmin {
		return ErrForbidden
	}
	lead, err := s.Repo.GetByID(id)
	if err != nil {
		return err
	}
	if lead == nil {
		return ErrLeadNotFound
	}
	if _, err := s.checkAssignee(lead, assigneeID); err != nil {
		return err
	}
	if err := s.Repo.UpdateOwner(id, assigneeID); err != nil {
		return err
	}
	log.Printf("[lead][assign] lead=%d owner %d -> %d by user=%d", id, lead.OwnerID, assigneeID, userID)
	s.notifyLeadBoard(lead)
	return nil
}

// TakeLead — «Взять себе»: сотрудник забирает ничей лид своего филиала.
func (s *LeadService) TakeLead(id, userID, roleID int) error {
	if authz.IsReadOnly(roleID) {
		return ErrReadOnly
	}
	if !authz.CanWorkWithLeads(roleID) {
		return ErrForbidden
	}
	lead, err := s.Repo.GetByID(id)
	if err != nil {
		return err
	}
	if lead == nil {
		return ErrLeadNotFound
	}
	scope, err := resolveLeadScope(userID, roleID, s.UserRepo)
	if err != nil {
		return err
	}
	if !leadMatchesScope(scope, lead) {
		return ErrForbidden
	}
	if lead.OwnerID == userID {
		return nil
	}
	if !s.ownerIsFree(lead.OwnerID) {
		return ErrLeadTaken
	}
	ok, err := s.Repo.ClaimOwner(id, userID, lead.OwnerID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrLeadTaken
	}
	log.Printf("[lead][take] lead=%d owner %d -> %d", id, lead.OwnerID, userID)
	s.notifyLeadBoard(lead)
	return nil
}

// Assignees — кому можно передать лид: работающие сотрудники, которые
// работают с лидами и видят этот лид. Руководство и админ — в конце списка.
func (s *LeadService) Assignees(id, roleID int) ([]LeadAssignee, error) {
	if roleID != authz.RoleManagement && roleID != authz.RoleSystemAdmin {
		return nil, ErrForbidden
	}
	lead, err := s.Repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if lead == nil {
		return nil, ErrLeadNotFound
	}
	if s.UserRepo == nil {
		return []LeadAssignee{}, nil
	}
	users, err := s.UserRepo.List(2000, 0) // только активные
	if err != nil {
		return nil, err
	}
	out := []LeadAssignee{}
	for _, u := range users {
		if u == nil || !u.IsVerified || !authz.CanWorkWithLeads(u.RoleID) {
			continue
		}
		elevated := authz.IsElevated(u.RoleID)
		if !elevated && lead.BranchID != nil && (u.BranchID == nil || *u.BranchID != *lead.BranchID) {
			continue
		}
		out = append(out, LeadAssignee{ID: u.ID, Name: userDisplayName(u), RoleID: u.RoleID, BranchID: u.BranchID, IsElevated: elevated})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsElevated != out[j].IsElevated {
			return !out[i].IsElevated
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// ReleaseLeadsOf снимает ответственного с рабочих лидов сотрудника, которого
// заблокировали, лишили подтверждения или удалили. Лиды остаются на своих
// этапах и в своём филиале — их видят и забирают себе коллеги.
func (s *LeadService) ReleaseLeadsOf(userID int) (int, error) {
	if s.Repo == nil || userID <= 0 {
		return 0, nil
	}
	count, funnels, err := s.Repo.ReleaseOwner(userID)
	if err != nil {
		return count, err
	}
	if count > 0 {
		log.Printf("[lead][release] user=%d leads=%d funnels=%v", userID, count, funnels)
	}
	if s.BoardNotifier != nil {
		for _, f := range funnels {
			s.BoardNotifier.NotifyBoardChanged(f)
		}
	}
	return count, nil
}

func (s *LeadService) notifyLeadBoard(lead *models.Leads) {
	if s.BoardNotifier != nil && lead != nil && lead.FunnelID != nil {
		s.BoardNotifier.NotifyBoardChanged(*lead.FunnelID)
	}
}
