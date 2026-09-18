package repositories

import (
	"strings"
	"testing"
)

// Обратная связь заказчика 18.09.2026: выделенный номер отдела контроля
// качества (жалобы, претензии) не должен попадать в общий пул лидов филиалов.
// Правило: лид без отдела виден всем, лид своего отдела виден, лид ЧУЖОГО
// закрытого отдела — нет.
func TestPrivateDepartmentWhere_HidesOtherPrivateDepartments(t *testing.T) {
	dept := 6
	args := []any{}
	cond, next := privateDepartmentWhere("l", &dept, &args, 1)

	for _, want := range []string{"l.department_id IS NULL", "l.department_id = $1", "COALESCE(pd.is_private, FALSE)"} {
		if !strings.Contains(cond, want) {
			t.Fatalf("expected %q in condition, got %s", want, cond)
		}
	}
	if len(args) != 1 || args[0] != dept {
		t.Fatalf("unexpected args: %#v", args)
	}
	if next != 2 {
		t.Fatalf("placeholder index must advance, got %d", next)
	}
}

// У смотрящего может не быть отдела — тогда ему видны только лиды без отдела
// и лиды открытых отделов.
func TestPrivateDepartmentWhere_WithoutViewerDepartment(t *testing.T) {
	args := []any{}
	cond, next := privateDepartmentWhere("l", nil, &args, 3)
	if strings.Contains(cond, "$") {
		t.Fatalf("condition must not add placeholders without a viewer department, got %s", cond)
	}
	if len(args) != 0 || next != 3 {
		t.Fatalf("nothing must be consumed: args=%#v next=%d", args, next)
	}
}

// Нумерация плейсхолдеров у вызывающих начинается не всегда с $1
// (buildLeadListWhere вызывается и с startAt=2).
func TestPrivateDepartmentWhere_RespectsPlaceholderOffset(t *testing.T) {
	dept := 6
	args := []any{}
	cond, next := privateDepartmentWhere("l", &dept, &args, 4)
	if !strings.Contains(cond, "l.department_id = $4") {
		t.Fatalf("expected placeholder $4, got %s", cond)
	}
	if next != 5 {
		t.Fatalf("expected next index 5, got %d", next)
	}
}

// Правило должно попадать в общий WHERE списка лидов только когда включено.
func TestBuildLeadListWhere_PrivateDepartmentsOptIn(t *testing.T) {
	dept := 6
	where, _ := buildLeadListWhere(LeadListFilter{HidePrivateDepartments: true, ViewerDepartmentID: &dept}, 1)
	if !strings.Contains(where, "is_private") {
		t.Fatalf("expected the private-department rule in where: %s", where)
	}

	where, _ = buildLeadListWhere(LeadListFilter{}, 1)
	if strings.Contains(where, "is_private") {
		t.Fatalf("rule must stay off by default (admin/management see everything): %s", where)
	}
}
