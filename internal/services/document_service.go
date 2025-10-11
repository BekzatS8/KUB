package services

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
	"turcompany/internal/models"
	"turcompany/internal/pdf"
	"turcompany/internal/repositories"
)

type DocumentService struct {
	Repo     *repositories.DocumentRepository
	LeadRepo *repositories.LeadRepository
	DealRepo *repositories.DealRepository
	smsRepo  *repositories.SMSConfirmationRepository
	pdfGen   pdf.Generator
	basePath string
}

func NewDocumentService(
	repo *repositories.DocumentRepository,
	leadRepo *repositories.LeadRepository,
	dealRepo *repositories.DealRepository,
	smsRepo *repositories.SMSConfirmationRepository,
	basePath string,
) *DocumentService {
	return &DocumentService{
		Repo:     repo,
		LeadRepo: leadRepo,
		DealRepo: dealRepo,
		smsRepo:  smsRepo,
		pdfGen:   pdf.NewDocumentGenerator(),
		basePath: basePath,
	}
}

func (s *DocumentService) CreateDocumentFromLead(leadID int, docType string) (*models.Document, error) {
	// Проверяем существование лида
	lead, err := s.LeadRepo.GetByID(leadID)
	if err != nil {
		return nil, fmt.Errorf("получение lead: %w", err)
	}

	// Получаем или создаем сделку для этого лида
	deal, err := s.DealRepo.GetByLeadID(leadID)
	if err != nil {
		// Если сделки нет, создаем новую
		newDeal := &models.Deals{
			LeadID:    leadID,
			Status:    "new",
			CreatedAt: time.Now(),
		}
		dealID, err := s.DealRepo.Create(newDeal)
		if err != nil {
			return nil, fmt.Errorf("создание сделки для лида: %w", err)
		}
		newDeal.ID = int(dealID)
		deal = newDeal
	}

	// Создаем структуру директорий (путь уже абсолютный из конструктора)
	docDir := filepath.Join(s.basePath, fmt.Sprintf("deal_%d", deal.ID))
	if err := os.MkdirAll(docDir, 0755); err != nil {
		return nil, fmt.Errorf("создание директории: %w", err)
	}

	// Формируем имя файла и путь
	fileName := fmt.Sprintf("%s_%s_%s.pdf",
		lead.Title,
		docType,
		time.Now().Format("20060102_150405"),
	)
	filePath := filepath.Join(docDir, fileName)

	// Сохраняем относительный путь в БД для переносимости
	relPath, err := filepath.Rel(s.basePath, filePath)
	if err != nil {
		return nil, fmt.Errorf("создание относительного пути: %w", err)
	}

	// Создаем документ с относительным путем
	doc := &models.Document{
		DealID:   int64(deal.ID),
		DocType:  docType,
		FilePath: filepath.Join("document_storage", relPath), // Сохраняем относительный путь
		Status:   "new",
	}

	// При генерации PDF используем абсолютный путь
	switch docType {
	case "contract":
		err = s.pdfGen.GenerateContract(pdf.ContractData{
			LeadTitle:    lead.Title,
			DealID:       deal.ID,
			Amount:       deal.Amount,
			Currency:     deal.Currency,
			CreatedAt:    time.Now(),
			DocumentPath: filePath, // Используем абсолютный путь
		})
	case "invoice":
		err = s.pdfGen.GenerateInvoice(pdf.InvoiceData{
			LeadTitle:    lead.Title,
			DealID:       deal.ID,
			Amount:       deal.Amount,
			Currency:     deal.Currency,
			CreatedAt:    time.Now(),
			DocumentPath: filePath, // Используем абсолютный путь
		})
	default:
		return nil, fmt.Errorf("неизвестный тип документа: %s", docType)
	}

	if err != nil {
		return nil, fmt.Errorf("генерация PDF: %w", err)
	}

	id, err := s.Repo.Create(doc)
	if err != nil {
		return nil, fmt.Errorf("сохранение документа: %w", err)
	}

	doc.ID = id
	return doc, nil
}

// Существующие методы остаются без изменений
func (s *DocumentService) CreateDocument(doc *models.Document) (int64, error) {
	if doc.FilePath == "" {
		return 0, fmt.Errorf("путь к файлу не указан")
	}

	// Создаем директорию для файла, если она не существует
	dir := filepath.Dir(doc.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, fmt.Errorf("создание директории для документа: %w", err)
	}

	return s.Repo.Create(doc)
}

func (s *DocumentService) GetDocument(id int64) (*models.Document, error) {
	return s.Repo.GetByID(id)
}

func (s *DocumentService) ListDocumentsByDeal(dealID int64) ([]*models.Document, error) {
	return s.Repo.ListDocumentsByDeal(dealID)
}

func (s *DocumentService) DeleteDocument(id int64) error {
	doc, err := s.Repo.GetByID(id)
	if err != nil {
		return err
	}

	// Удаляем файл, если он существует
	if doc.FilePath != "" {
		if err := os.Remove(doc.FilePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("удаление файла документа: %w", err)
		}
	}

	return s.Repo.Delete(id)
}
func (s *DocumentService) ListDocuments(limit, offset int) ([]*models.Document, error) {
	return s.Repo.ListDocuments(limit, offset)
}
