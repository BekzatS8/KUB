package services

import (
	"context"
	"errors"
	"slices"
	"testing"
)

type templateDeptRepoStub struct {
	mapping map[string][]string
	saved   map[string][]string
	listErr error
}

func (s *templateDeptRepoStub) List(context.Context) (map[string][]string, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.mapping, nil
}

func (s *templateDeptRepoStub) ListDocTypesByScope(_ context.Context, scope string) ([]string, error) {
	out := []string{}
	for docType, scopes := range s.mapping {
		for _, sc := range scopes {
			if sc == scope {
				out = append(out, docType)
			}
		}
	}
	return out, nil
}

func (s *templateDeptRepoStub) SetForDocType(_ context.Context, docType string, scopes []string) error {
	if s.saved == nil {
		s.saved = map[string][]string{}
	}
	s.saved[docType] = scopes
	return nil
}

// Отдел видит только свои шаблоны — по ним он и генерирует документы клиентов.
func TestListDocumentTypesWithDepartmentsFiltersByScope(t *testing.T) {
	repo := &templateDeptRepoStub{mapping: map[string][]string{
		"contract_language_courses":    {"visa"},
		"contract_paid_50_50_ru": {"sales"},
		"receipt_refund_full":    {"sales", "legal"},
	}}
	svc := &DocumentService{TemplateDeptRepo: repo}
	ctx := context.Background()

	visa, err := svc.ListDocumentTypesWithDepartments(ctx, "visa")
	if err != nil {
		t.Fatal(err)
	}
	if len(visa) != 1 || visa[0].DocType != "contract_language_courses" {
		t.Fatalf("визовый отдел: ожидался только contract_language_courses, получено %+v", docTypesOf(visa))
	}

	// шаблон в двух отделах виден обоим
	for _, scope := range []string{"sales", "legal"} {
		got, err := svc.ListDocumentTypesWithDepartments(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(docTypesOf(got), "receipt_refund_full") {
			t.Fatalf("%s: расписка должна быть видна, получено %+v", scope, docTypesOf(got))
		}
	}

	// у отдела без шаблонов — пусто, а не весь реестр
	hr, err := svc.ListDocumentTypesWithDepartments(ctx, "hr")
	if err != nil {
		t.Fatal(err)
	}
	if len(hr) != 0 {
		t.Fatalf("кадры: ожидалось пусто, получено %+v", docTypesOf(hr))
	}
}

// Без scope — весь реестр с проставленными отделами (экран настроек админа).
func TestListDocumentTypesWithDepartmentsWithoutScope(t *testing.T) {
	repo := &templateDeptRepoStub{mapping: map[string][]string{
		"contract_language_courses": {"visa"},
	}}
	svc := &DocumentService{TemplateDeptRepo: repo}

	all, err := svc.ListDocumentTypesWithDepartments(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(ListDocumentTypeSpecs()) {
		t.Fatalf("без scope ожидался весь реестр (%d), получено %d", len(ListDocumentTypeSpecs()), len(all))
	}
	for _, spec := range all {
		if spec.DocType == "contract_language_courses" {
			if len(spec.Departments) != 1 || spec.Departments[0] != "visa" {
				t.Fatalf("отделы не проставлены: %+v", spec.Departments)
			}
		}
		// нераспределённый шаблон приходит с пустым списком, а не пропадает
		if spec.DocType == "contract_paid_50_50_ru" && len(spec.Departments) != 0 {
			t.Fatalf("ожидался нераспределённый шаблон, получено %+v", spec.Departments)
		}
	}
}

// Пока раскладка не подключена (нет миграции) — показываем реестр целиком,
// иначе у всех отделов было бы пусто.
func TestListDocumentTypesWithoutRepoReturnsWholeRegistry(t *testing.T) {
	svc := &DocumentService{}
	all, err := svc.ListDocumentTypesWithDepartments(context.Background(), "visa")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(ListDocumentTypeSpecs()) {
		t.Fatalf("ожидался весь реестр, получено %d", len(all))
	}
}

func TestSetTemplateDepartmentsRejectsUnknownDocType(t *testing.T) {
	svc := &DocumentService{TemplateDeptRepo: &templateDeptRepoStub{}}
	err := svc.SetTemplateDepartments(context.Background(), "no_such_template", []string{"sales"})
	if !errors.Is(err, ErrDocumentTypeUnknown) {
		t.Fatalf("ожидался ErrDocumentTypeUnknown, получено %v", err)
	}
}

func TestSetTemplateDepartmentsSaves(t *testing.T) {
	repo := &templateDeptRepoStub{}
	svc := &DocumentService{TemplateDeptRepo: repo}
	if err := svc.SetTemplateDepartments(context.Background(), "contract_language_courses", []string{"visa", "sales"}); err != nil {
		t.Fatal(err)
	}
	if got := repo.saved["contract_language_courses"]; len(got) != 2 {
		t.Fatalf("ожидалось сохранение двух отделов, получено %+v", got)
	}
}

func docTypesOf(specs []DocumentTypeSpec) []string {
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.DocType)
	}
	return out
}
