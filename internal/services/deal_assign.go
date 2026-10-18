package services

import (
	"log"

	"turcompany/internal/authz"
)

// Ответственный за сделку: «Взять себе» для ничьей сделки и освобождение
// сделок уволенного сотрудника (как у лидов, см. lead_assign.go).

// TakeDeal — «Взять себе»: сотрудник забирает ничью сделку своего филиала
// (без ответственного, ответственный уволен/не подтверждён или сделка
// «припаркована» на руководстве).
func (s *DealService) TakeDeal(id, userID, roleID int) error {
	if authz.IsReadOnly(roleID) {
		return ErrReadOnly
	}
	if !authz.CanWorkWithLeads(roleID) {
		return ErrForbidden
	}
	deal, err := s.Repo.GetByID(id)
	if err != nil {
		return err
	}
	if deal == nil {
		return ErrDealNotFound
	}
	scope, err := resolveDealScope(userID, roleID, s.UserRepo)
	if err != nil {
		return err
	}
	if !dealMatchesScope(scope, deal) {
		return ErrForbidden
	}
	if deal.OwnerID == userID {
		return nil
	}
	if !ownerIsFreeFor(s.UserRepo, deal.OwnerID) {
		return ErrLeadTaken
	}
	ok, err := s.Repo.ClaimOwner(id, userID, deal.OwnerID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrLeadTaken
	}
	log.Printf("[deal][take] deal=%d owner %d -> %d", id, deal.OwnerID, userID)
	if deal.FunnelID != nil {
		s.notifyBoard(*deal.FunnelID)
	}
	return nil
}

// ReleaseDealsOf снимает ответственного с рабочих сделок сотрудника, которого
// заблокировали, лишили подтверждения или удалили.
func (s *DealService) ReleaseDealsOf(userID int) (int, error) {
	if s.Repo == nil || userID <= 0 {
		return 0, nil
	}
	count, funnels, err := s.Repo.ReleaseOwner(userID)
	if err != nil {
		return count, err
	}
	if count > 0 {
		log.Printf("[deal][release] user=%d deals=%d funnels=%v", userID, count, funnels)
	}
	for _, f := range funnels {
		s.notifyBoard(f)
	}
	return count, nil
}
