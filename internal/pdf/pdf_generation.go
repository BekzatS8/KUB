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
	g.addUTF8Font(pdf)
	pdf.SetFont(g.fontName, "", 14)
	pdf.AddPage()

	// заголовок
	pdf.SetFont(g.fontName, "B", 16)
	pdf.SetY(20)
	center := (210 - pdf.GetStringWidth("ДОГОВОР")) / 2
	if center < 10 {
		center = 10
	}
	pdf.SetX(center)
	pdf.Cell(40, 10, "ДОГОВОР")
	pdf.Ln(20)

	// контент
	g.addLines(pdf, []string{
		fmt.Sprintf("Номер договора: %d", data.DealID),
		fmt.Sprintf("Клиент: %s", data.LeadTitle),
		fmt.Sprintf("Сумма: %s %s", data.Amount, data.Currency),
		fmt.Sprintf("Дата создания: %s", data.CreatedAt.Format("02.01.2006")),
	})

	if err := pdf.OutputFileAndClose(absPath); err != nil {
		return "", err
	}

	// для БД храним относительный путь — только имя файла
	return "/" + filepath.ToSlash(filepath.Base(absPath)), nil
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
