package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jung-kurt/gofpdf"
)

// Generator — интерфейс (удобно мокать в тестах)
type Generator interface {
	GenerateContract(data ContractData) (string, error)
	GenerateInvoice(data InvoiceData) (string, error)
}

// DocumentGenerator — реализация
type DocumentGenerator struct {
	RootDir  string // корень хранения, например "./files"
	FontPath string // путь до TTF, например "assets/fonts/DejaVuSans.ttf"
	fontName string // внутреннее имя шрифта в PDF
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

func NewDocumentGenerator(rootDir, fontPath string) *DocumentGenerator {
	return &DocumentGenerator{
		RootDir:  filepath.Clean(rootDir),
		FontPath: fontPath,
		fontName: "DejaVu",
	}
}

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

	// Короткая вводная (переносы строк)
	pdf.SetFont(g.fontName, "", 11)
	intro := "Стороны договорились о предоставлении услуг в соответствии с условиями настоящего договора. " +
		"Подробные условия, сроки и порядок расчётов определяются Соглашением и Приложениями к нему."
	pdf.MultiCell(0, 6, intro, "", "L", false)
	pdf.Ln(2)
	g.hr(pdf)

	// ===== Условия (краткие, чтобы документ выглядел «настоящим»)
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
	// Левые подписи (Исполнитель)
	pdf.SetFont(g.fontName, "", 11)
	pdf.CellFormat(80, 6, "Исполнитель", "", 0, "L", false, 0, "")
	pdf.CellFormat(30, 6, "", "", 0, "L", false, 0, "")
	pdf.CellFormat(80, 6, "Заказчик", "", 1, "L", false, 0, "")

	// Линии для подписи
	pdf.SetLineWidth(0.3)
	// Исполнитель
	pdf.Line(20, lineY+10, 100, lineY+10) // подпись
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
	return "/" + filepath.ToSlash(filepath.Base(absPath)), nil
}

// === helpers (добавь ниже в том же файле) ===
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
	return "/" + filepath.ToSlash(filepath.Base(absPath)), nil
}

// ===== helpers =====

func (g *DocumentGenerator) ensureTarget(filename string) (string, error) {
	if err := os.MkdirAll(g.RootDir, 0o755); err != nil {
		return "", fmt.Errorf("create files dir: %w", err)
	}
	filename = filepath.Base(filename) // безопасность
	return filepath.Join(g.RootDir, filename), nil
}

func (g *DocumentGenerator) addUTF8Font(pdf *gofpdf.Fpdf) {
	// AddUTF8Font принимает путь до TTF
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
