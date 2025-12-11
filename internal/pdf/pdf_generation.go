package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf"
)

// Generator — интерфейс (удобно мокать в тестах)
type Generator interface {
	GenerateContract(data ContractData) (string, error)
	GenerateInvoice(data InvoiceData) (string, error)
	// Теперь сюда передаём ИМЯ шаблона, а не полный путь
	GenerateFromTemplate(templateName string, placeholders map[string]string, filename string) (string, error)
}

// DocumentGenerator — реализация
type DocumentGenerator struct {
	RootDir      string // корень хранения PDF, например "./files"
	TemplatesDir string // корень шаблонов, например "./assets/templates"
	FontPath     string // путь до TTF, например "assets/fonts/DejaVuSans.ttf"
	fontName     string // внутреннее имя шрифта в PDF
}

type ContractData struct {
	LeadTitle string
	DealID    int
	Amount    string
	Currency  string
	CreatedAt time.Time
	Filename  string // имя файла (без путей); если пусто — сгенерируем
}

type InvoiceData struct {
	LeadTitle string
	DealID    int
	Amount    string
	Currency  string
	CreatedAt time.Time
	Filename  string
}

// NewDocumentGenerator создаёт генератор
// rootDir      — куда складывать PDF (например, "files")
// templatesDir — откуда брать .txt шаблоны (например, "assets/templates")
// fontPath     — путь к TTF-шрифту (например, "assets/fonts/DejaVuSans.ttf")
func NewDocumentGenerator(rootDir, templatesDir, fontPath string) *DocumentGenerator {
	return &DocumentGenerator{
		RootDir:      filepath.Clean(rootDir),
		TemplatesDir: filepath.Clean(templatesDir),
		FontPath:     fontPath,
		fontName:     "DejaVu",
	}
}

// ======================= CONTRACT =======================

func (g *DocumentGenerator) GenerateContract(data ContractData) (string, error) {
	filename := data.Filename
	if filename == "" {
		filename = fmt.Sprintf("contract_deal_%d.pdf", data.DealID)
	}
	absPath, err := g.ensureTarget(filename)
	if err != nil {
		return "", err
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(fmt.Sprintf("Договор №%d", data.DealID), false)
	pdf.SetAuthor("KUB SRM", false)
	pdf.SetMargins(20, 20, 20)
	pdf.SetAutoPageBreak(true, 20)

	g.addUTF8Font(pdf)
	pdf.AddPage()

	// ===== Заголовок
	pdf.SetFont(g.fontName, "B", 18)
	pdf.CellFormat(0, 10, "ДОГОВОР", "", 1, "C", false, 0, "")

	pdf.SetFont(g.fontName, "", 12)
	sub := fmt.Sprintf("№ KUB-%06d  от  %s",
		data.DealID,
		data.CreatedAt.Format("02.01.2006"),
	)
	pdf.CellFormat(0, 7, sub, "", 1, "C", false, 0, "")
	g.hr(pdf)

	pdf.Ln(3)

	// ===== Стороны
	g.sectionTitle(pdf, "Стороны")
	g.kvLine(pdf, "Исполнитель", "Ваша компания")
	g.kvLine(pdf, "Заказчик", data.LeadTitle)
	pdf.Ln(2)
	g.hr(pdf)

	// ===== Предмет и сумма
	g.sectionTitle(pdf, "Предмет и сумма")
	g.kvLine(pdf, "Номер договора", fmt.Sprintf("%d", data.DealID))
	g.kvLine(pdf, "Сумма", fmt.Sprintf("%s %s", data.Amount, data.Currency))
	pdf.Ln(1)

	// Короткая вводная
	pdf.SetFont(g.fontName, "", 11)
	intro := "Стороны договорились о предоставлении услуг в соответствии с условиями настоящего договора. " +
		"Подробные условия, сроки и порядок расчётов определяются Соглашением и Приложениями к нему."
	pdf.MultiCell(0, 6, intro, "", "L", false)
	pdf.Ln(2)
	g.hr(pdf)

	// ===== Условия
	g.sectionTitle(pdf, "Основные условия")
	pdf.SetFont(g.fontName, "", 11)
	terms := []string{
		"1. Срок оказания услуг определяется календарным планом и согласуется Сторонами.",
		"2. Заказчик обязуется оплатить услуги Исполнителя в размере, указанном выше.",
		"3. Документ вступает в силу с даты подписания Сторонами.",
		"4. Все споры разрешаются путём переговоров, при недостижении согласия — в соответствии с применимым законодательством.",
	}
	for _, t := range terms {
		pdf.MultiCell(0, 6, t, "", "L", false)
	}
	pdf.Ln(2)
	g.hr(pdf)

	// ===== Подписи
	g.sectionTitle(pdf, "Подписи")
	pdf.Ln(6)

	lineY := pdf.GetY()
	pdf.SetFont(g.fontName, "", 11)
	pdf.CellFormat(80, 6, "Исполнитель", "", 0, "L", false, 0, "")
	pdf.CellFormat(30, 6, "", "", 0, "L", false, 0, "")
	pdf.CellFormat(80, 6, "Заказчик", "", 1, "L", false, 0, "")

	// Линии для подписи
	pdf.SetLineWidth(0.3)
	// Исполнитель
	pdf.Line(20, lineY+10, 100, lineY+10)
	pdf.SetY(lineY + 12)
	pdf.SetX(20)
	pdf.Cell(80, 5, "(подпись, ФИО)")
	// Заказчик
	pdf.SetY(lineY + 6)
	pdf.SetX(130)
	pdf.Line(130, lineY+10, 190, lineY+10)
	pdf.SetY(lineY + 12)
	pdf.SetX(130)
	pdf.Cell(80, 5, "(подпись, ФИО)")

	// ===== Нумерация страниц
	pdf.AliasNbPages("")
	pdf.SetFooterFunc(func() {
		pdf.SetY(-15)
		pdf.SetFont(g.fontName, "", 10)
		pdf.CellFormat(0, 10,
			fmt.Sprintf("Стр. %d/{nb}", pdf.PageNo()),
			"", 0, "C", false, 0, "",
		)
	})

	if err := pdf.OutputFileAndClose(absPath); err != nil {
		return "", err
	}
	return g.relativePath(absPath), nil
}

// ======================= INVOICE =======================

func (g *DocumentGenerator) GenerateInvoice(data InvoiceData) (string, error) {
	filename := data.Filename
	if filename == "" {
		filename = fmt.Sprintf("invoice_deal_%d.pdf", data.DealID)
	}
	absPath, err := g.ensureTarget(filename)
	if err != nil {
		return "", err
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	g.addUTF8Font(pdf)
	pdf.SetFont(g.fontName, "", 14)
	pdf.SetMargins(20, 20, 20)
	pdf.SetAutoPageBreak(true, 20)
	pdf.AddPage()

	pdf.SetFont(g.fontName, "B", 16)
	pdf.SetY(20)
	center := (210 - pdf.GetStringWidth("СЧЕТ")) / 2
	if center < 10 {
		center = 10
	}
	pdf.SetX(center)
	pdf.Cell(40, 10, "СЧЕТ")
	pdf.Ln(20)

	g.addLines(pdf, []string{
		fmt.Sprintf("Номер счета: %d", data.DealID),
		fmt.Sprintf("Клиент: %s", data.LeadTitle),
		fmt.Sprintf("Сумма к оплате: %s %s", data.Amount, data.Currency),
		fmt.Sprintf("Дата выставления: %s", data.CreatedAt.Format("02.01.2006")),
	})

	if err := pdf.OutputFileAndClose(absPath); err != nil {
		return "", err
	}
	return g.relativePath(absPath), nil
}

// ======================= TEMPLATES =======================

// resolveTemplatePath — находит реальный путь до шаблона
func (g *DocumentGenerator) resolveTemplatePath(templateName string) (string, error) {
	if templateName == "" {
		return "", fmt.Errorf("empty template name")
	}

	// Если пришёл абсолютный путь — используем как есть
	if filepath.IsAbs(templateName) {
		if _, err := os.Stat(templateName); err != nil {
			return "", fmt.Errorf("template not found %s: %w", templateName, err)
		}
		return templateName, nil
	}

	base := g.TemplatesDir
	if base == "" {
		base = "assets/templates/txt"
	}
	path := filepath.Join(base, templateName)

	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("template not found %s: %w", path, err)
	}
	return path, nil
}

// GenerateFromTemplate — универсальная генерация PDF из текстового шаблона с плейсхолдерами {{KEY}}
func (g *DocumentGenerator) GenerateFromTemplate(
	templateName string,
	placeholders map[string]string,
	filename string,
) (string, error) {
	path, err := g.resolveTemplatePath(templateName)
	if err != nil {
		return "", err
	}

	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read template %s: %w", path, err)
	}
	content := string(contentBytes)

	// Заменяем {{KEY}} на значения
	for k, v := range placeholders {
		ph := fmt.Sprintf("{{%s}}", k)
		content = strings.ReplaceAll(content, ph, v)
	}

	if filename == "" {
		filename = fmt.Sprintf("doc_%d.pdf", time.Now().Unix())
	}

	absPath, err := g.ensureTarget(filename)
	if err != nil {
		return "", err
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(filename, false)
	pdf.SetAuthor("KUB SRM", false)
	pdf.SetMargins(20, 20, 20)
	pdf.SetAutoPageBreak(true, 20)

	g.addUTF8Font(pdf)
	pdf.AddPage()
	pdf.SetFont(g.fontName, "", 11)

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		pdf.MultiCell(0, 5, line, "", "L", false)
	}

	if err := pdf.OutputFileAndClose(absPath); err != nil {
		return "", err
	}

	// возвращаем относительный путь (как раньше)
	return g.relativePath(absPath), nil
}

// ======================= HELPERS =======================

func (g *DocumentGenerator) sectionTitle(pdf *gofpdf.Fpdf, s string) {
	pdf.SetFont(g.fontName, "B", 12)
	pdf.CellFormat(0, 7, s, "", 1, "L", false, 0, "")
	pdf.SetFont(g.fontName, "", 11)
}

func (g *DocumentGenerator) kvLine(pdf *gofpdf.Fpdf, key, val string) {
	pdf.SetFont(g.fontName, "B", 11)
	pdf.CellFormat(45, 6, key+":", "", 0, "L", false, 0, "")
	pdf.SetFont(g.fontName, "", 11)
	pdf.CellFormat(0, 6, val, "", 1, "L", false, 0, "")
}

func (g *DocumentGenerator) hr(pdf *gofpdf.Fpdf) {
	y := pdf.GetY() + 1.5
	pdf.SetLineWidth(0.2)
	pdf.Line(20, y, 190, y)
	pdf.SetY(y + 2)
}

func (g *DocumentGenerator) ensureTarget(filename string) (string, error) {
	pdfDir := g.pdfDir()
	if err := os.MkdirAll(pdfDir, 0o755); err != nil {
		return "", fmt.Errorf("create files dir: %w", err)
	}
	filename = filepath.Base(filename) // безопасность
	return filepath.Join(pdfDir, filename), nil
}

func (g *DocumentGenerator) pdfDir() string {
	if g.RootDir == "" {
		g.RootDir = "files"
	}
	return filepath.Join(g.RootDir, "pdf")
}

func (g *DocumentGenerator) relativePath(absPath string) string {
	filename := filepath.Base(absPath)
	return "/" + filepath.ToSlash(filepath.Join("pdf", filename))
}

func (g *DocumentGenerator) addUTF8Font(pdf *gofpdf.Fpdf) {
	pdf.AddUTF8Font(g.fontName, "", g.FontPath)
	pdf.AddUTF8Font(g.fontName, "B", g.FontPath)
}

func (g *DocumentGenerator) addLines(pdf *gofpdf.Fpdf, lines []string) {
	pdf.SetFont(g.fontName, "", 12)
	left := 20.0
	for _, line := range lines {
		pdf.SetX(left)
		pdf.Cell(0, 10, line)
		pdf.Ln(15)
	}
}
