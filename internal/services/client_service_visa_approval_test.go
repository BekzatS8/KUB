package services

import (
	"errors"
	"testing"

	"turcompany/internal/authz"
	"turcompany/internal/models"
)

// Визовый отдел (visa, role_id=60) НЕ может редактировать клиентов напрямую:
// и Update (PUT), и Patch (PATCH) обязаны идти через подтверждение администратора
// (feed_events). Guard срабатывает до обращения к репозиторию, поэтому пустой
// ClientService достаточен. Одобренная заявка применяется с RoleSystemAdmin и
// этот guard не затрагивает (см. feed_event_service.applyEvent).
func TestClientEdit_VisaRequiresAdminApproval(t *testing.T) {
	s := &ClientService{}

	if err := s.Update(&models.Client{ID: 1}, 100, authz.RoleVisa); !errors.Is(err, ErrClientEditNeedsApproval) {
		t.Fatalf("visa direct Update must require approval, got %v", err)
	}

	if _, err := s.Patch(1, map[string]any{"name": "x"}, 100, authz.RoleVisa); !errors.Is(err, ErrClientEditNeedsApproval) {
		t.Fatalf("visa direct Patch must require approval, got %v", err)
	}

	// ErrClientEditNeedsApproval оборачивает ErrForbidden → маппится в 403 в хендлере.
	if !errors.Is(ErrClientEditNeedsApproval, ErrForbidden) {
		t.Fatal("ErrClientEditNeedsApproval must wrap ErrForbidden for 403 mapping")
	}
}

// Партнёрский отдел (partner, role_id=70) — как и visa — НЕ может редактировать
// клиентов напрямую: по ТЗ «редактировать клиентов, но требует подтверждения
// админа». И Update (PUT), и Patch (PATCH) обязаны идти через Ленту (feed_events).
func TestClientEdit_PartnerRequiresAdminApproval(t *testing.T) {
	s := &ClientService{}

	if err := s.Update(&models.Client{ID: 1}, 100, authz.RolePartner); !errors.Is(err, ErrClientEditNeedsApproval) {
		t.Fatalf("partner direct Update must require approval, got %v", err)
	}

	if _, err := s.Patch(1, map[string]any{"name": "x"}, 100, authz.RolePartner); !errors.Is(err, ErrClientEditNeedsApproval) {
		t.Fatalf("partner direct Patch must require approval, got %v", err)
	}
}
