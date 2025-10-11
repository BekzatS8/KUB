package pdf

import (
	"fmt"
	"github.com/jung-kurt/gofpdf"
	"time"
)

// Generator интерфейс для создания различных типов PDF документов
type Generator interface {
	GenerateContract(data ContractData) error
	GenerateInvoice(data InvoiceData) error
}

// DocumentGenerator основная структура для генерации PDF
type DocumentGenerator struct {
	pdf *gofpdf.Fpdf
}

// ContractData структура данных для контракта
type ContractData struct {
	LeadTitle    string
	DealID       int
	Amount       string
	Currency     string
	CreatedAt    time.Time
	DocumentPath string
}

// InvoiceData структура данных для счета
type InvoiceData struct {
	LeadTitle    string
	DealID       int
	Amount       string
	Currency     string
	CreatedAt    time.Time
	DocumentPath string
}

// NewDocumentGenerator создает новый генератор PDF
func NewDocumentGenerator() *DocumentGenerator {
	return &DocumentGenerator{}
}

func (g *DocumentGenerator) GenerateContract(data ContractData) error {
	// Создаем PDF с поддержкой UTF-8
	pdf := gofpdf.New("P", "mm", "A4", "")
	g.pdf = pdf

	// Устанавливаем шрифт с поддержкой UTF-8
	pdf.SetFont("Arial", "", 14)

	pdf.AddPage()

	// Заголовок
	pdf.SetFont("Arial", "B", 16)
	pdf.SetY(20)

	// Используем Cell вместо WriteAligned для UTF-8 текста
	pdf.SetX((210 - pdf.GetStringWidth("ДОГОВОР")) / 2)
	pdf.Cell(40, 10, "ДОГОВОР")
	pdf.Ln(20)

	// Информация о контракте
	g.addContractInfo(data)

	return pdf.OutputFileAndClose(data.DocumentPath)
}

func (g *DocumentGenerator) addContractInfo(data ContractData) {
	g.pdf.SetFont("Arial", "", 12)

	// Устанавливаем отступ слева
	leftMargin := 20.0
	g.pdf.SetX(leftMargin)

	// Добавляем информацию построчно
	lines := []string{
		fmt.Sprintf("Номер договора: %d", data.DealID),
		fmt.Sprintf("Клиент: %s", data.LeadTitle),
		fmt.Sprintf("Сумма: %s %s", data.Amount, data.Currency),
		fmt.Sprintf("Дата создания: %s", data.CreatedAt.Format("02.01.2006")),
	}

	for _, line := range lines {
		g.pdf.SetX(leftMargin)
		g.pdf.Cell(0, 10, line)
		g.pdf.Ln(15)
	}
}

// Аналогично обновляем GenerateInvoice
func (g *DocumentGenerator) GenerateInvoice(data InvoiceData) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	g.pdf = pdf

	pdf.SetFont("Arial", "", 14)

	pdf.AddPage()

	// Заголовок
	pdf.SetFont("Arial", "B", 16)
	pdf.SetY(20)

	pdf.SetX((210 - pdf.GetStringWidth("СЧЕТ")) / 2)
	pdf.Cell(40, 10, "СЧЕТ")
	pdf.Ln(20)

	g.addInvoiceInfo(data)

	return pdf.OutputFileAndClose(data.DocumentPath)
}

func (g *DocumentGenerator) addInvoiceInfo(data InvoiceData) {
	g.pdf.SetFont("Arial", "", 12)

	leftMargin := 20.0
	g.pdf.SetX(leftMargin)

	lines := []string{
		fmt.Sprintf("Номер счета: %d", data.DealID),
		fmt.Sprintf("Клиент: %s", data.LeadTitle),
		fmt.Sprintf("Сумма к оплате: %s %s", data.Amount, data.Currency),
		fmt.Sprintf("Дата выставления: %s", data.CreatedAt.Format("02.01.2006")),
	}

	for _, line := range lines {
		g.pdf.SetX(leftMargin)
		g.pdf.Cell(0, 10, line)
		g.pdf.Ln(15)
	}
}
