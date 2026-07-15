package handlers

import (
	"slices"
	"testing"

	"turcompany/internal/authz"
)

// Кто какие scope документов видит (ТЗ 04.07.2026, п.2.1). Табы «Документы
// отдела» / «Документы клиентов» сужают выдачу через ?scope=, поэтому набор
// разрешённых scope должен быть точным.
func TestAllowedDocumentScopes(t *testing.T) {
	tests := []struct {
		name   string
		roleID int
		want   []string
	}{
		{"кадры — только свой отдел", authz.RoleHR, []string{"hr"}},
		{"юристы — только свой отдел", authz.RoleLegal, []string{"legal"}},
		{"контроль качества — только свой отдел", authz.RoleControl, []string{"quality_control"}},
		{"продажи — свой отдел + документы по сделкам", authz.RoleSales, []string{"sales", "deal"}},
		{"визовый — свой отдел + документы по сделкам", authz.RoleVisa, []string{"visa", "deal"}},
		{"партнёрский — только свой отдел", authz.RolePartner, []string{"partner"}},
		{"админ — без ограничений", authz.RoleSystemAdmin, nil},
		{"руководство — без ограничений", authz.RoleManagement, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := allowedDocumentScopes(tt.roleID)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("allowedDocumentScopes(%d) = %v, ожидалось %v", tt.roleID, got, tt.want)
			}
		})
	}
}

// Ни одна роль не должна получить доступ к чужому отделу через ?scope=.
func TestAllowedDocumentScopesDoNotLeakForeignDepartments(t *testing.T) {
	foreign := map[int][]string{
		authz.RoleHR:      {"legal", "sales", "visa", "partner", "quality_control", "management", "deal"},
		authz.RoleLegal:   {"hr", "sales", "visa", "partner", "quality_control", "management", "deal"},
		authz.RoleSales:   {"hr", "legal", "visa", "partner", "quality_control", "management"},
		authz.RoleVisa:    {"hr", "legal", "sales", "partner", "quality_control", "management"},
		authz.RolePartner: {"hr", "legal", "sales", "visa", "quality_control", "management", "deal"},
		authz.RoleControl: {"hr", "legal", "sales", "visa", "partner", "management", "deal"},
	}

	for roleID, scopes := range foreign {
		allowed := allowedDocumentScopes(roleID)
		if allowed == nil {
			t.Fatalf("роль %d не должна быть без ограничений", roleID)
		}
		for _, s := range scopes {
			if slices.Contains(allowed, s) {
				t.Fatalf("роль %d получила доступ к чужому scope %q", roleID, s)
			}
		}
	}
}

// Каждый разрешённый scope должен быть валидным значением колонки documents.scope,
// иначе фильтр молча вернёт пустоту.
func TestAllowedDocumentScopesAreValidScopes(t *testing.T) {
	roles := []int{
		authz.RoleHR, authz.RoleLegal, authz.RoleControl,
		authz.RoleSales, authz.RoleVisa, authz.RolePartner,
	}
	for _, roleID := range roles {
		for _, s := range allowedDocumentScopes(roleID) {
			if !isAllowedDocumentScope(s) {
				t.Fatalf("роль %d: scope %q не проходит isAllowedDocumentScope", roleID, s)
			}
		}
	}
}
