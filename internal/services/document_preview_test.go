package services

import (
	"strings"
	"testing"
)

// Заглушка генератора: проверяем, что сервис просит ключи шаблона и подставляет
// человеческие подписи, а не {{КЛЮЧИ}}.
type previewGenStub struct {
	keys     []string
	gotPh    map[string]string
	gotTmpl  string
	rendered []byte
}

func (g *previewGenStub) GenerateDocxAndPDF(string, map[string]string, string) (string, string, error) {
	return "", "", nil
}
func (g *previewGenStub) GeneratePDF(string, map[string]string, string) (string, error) {
	return "", nil
}
func (g *previewGenStub) TemplatePlaceholderKeys(string) ([]string, error) { return g.keys, nil }
func (g *previewGenStub) RenderDocxToBytes(tmpl string, ph map[string]string) ([]byte, error) {
	g.gotTmpl = tmpl
	g.gotPh = ph
	return g.rendered, nil
}

func TestPreviewTemplateFillsHumanLabels(t *testing.T) {
	gen := &previewGenStub{
		keys:     []string{"CLIENT_FULL_NAME", "CLIENT_IIN", "TOTAL_AMOUNT_WORDS_KZ", "SOME_NEW_KEY"},
		rendered: []byte("PK-docx"),
	}
	svc := &DocumentService{DocxGen: gen}

	data, fileName, err := svc.PreviewTemplate("contract_paid_50_50_ru")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "PK-docx" {
		t.Fatalf("вернулись не байты шаблона: %q", data)
	}
	if fileName != "contract_paid_50_50_ru.docx" {
		t.Fatalf("имя файла: %q", fileName)
	}
	if got := gen.gotPh["CLIENT_FULL_NAME"]; got != "Фамилия Имя Отчество" {
		t.Fatalf("ФИО: ожидалась подпись, получено %q", got)
	}
	if got := gen.gotPh["TOTAL_AMOUNT_WORDS_KZ"]; got != "сома жазбаша" {
		t.Fatalf("каз. сумма: получено %q", got)
	}
	// незнакомый ключ не должен ронять предпросмотр — подставляем сам ключ
	if got := gen.gotPh["SOME_NEW_KEY"]; got != "SOME_NEW_KEY" {
		t.Fatalf("новый ключ: получено %q", got)
	}
	// в предпросмотре не должно остаться фигурных скобок
	for k, v := range gen.gotPh {
		if strings.Contains(v, "{{") {
			t.Fatalf("ключ %s подставлен как плейсхолдер: %q", k, v)
		}
	}
}

func TestPreviewTemplateUnknownDocType(t *testing.T) {
	svc := &DocumentService{DocxGen: &previewGenStub{}}
	if _, _, err := svc.PreviewTemplate("no_such"); err != ErrDocumentTypeUnknown {
		t.Fatalf("ожидался ErrDocumentTypeUnknown, получено %v", err)
	}
}
