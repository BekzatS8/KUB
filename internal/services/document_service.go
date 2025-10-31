package services

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/pdf"
	"turcompany/internal/repositories"
)

type DocumentService struct {
	DocRepo    *repositories.DocumentRepository
	LeadRepo   *repositories.LeadRepository
	DealRepo   *repositories.DealRepository
	SMSRepo    *repositories.SMSConfirmationRepository
	SignSecret string

	FilesRoot string        // корень хранения файлов (cfg.Files.RootDir)
	PDFGen    pdf.Generator // генератор PDF (internal/pdf)
}

func NewDocumentService(
	docRepo *repositories.DocumentRepository,
	leadRepo *repositories.LeadRepository,
	dealRepo *repositories.DealRepository,
	smsRepo *repositories.SMSConfirmationRepository,
	signSecret string,
	filesRoot string,
	pdfGen pdf.Generator,
) *DocumentService {
	return &DocumentService{
		DocRepo:    docRepo,
		LeadRepo:   leadRepo,
		DealRepo:   dealRepo,
		SMSRepo:    smsRepo,
		SignSecret: signSecret,
		FilesRoot:  filesRoot,
		PDFGen:     pdfGen,
	}
}

// ===== CRUD =====

func (s *DocumentService) CreateDocument(doc *models.Document, userID, roleID int) (int64, error) {
	if authz.IsReadOnly(roleID) {
		return 0, errors.New("read-only role")
	}
	if doc.DealID == 0 {
		return 0, errors.New("deal not found")
	}
	deal, err := s.DealRepo.GetByID(int(doc.DealID))
	if err != nil || deal == nil {
		return 0, errors.New("deal not found")
	}
	// Sales может создавать документ только по своей сделке
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return 0, errors.New("forbidden")
	}

	// по умолчанию
	if strings.TrimSpace(doc.Status) == "" {
		doc.Status = "draft"
	}

	// Нормализация входного пути (если он вдруг есть)
	filename := filepath.Base(strings.TrimSpace(doc.FilePath)) // "" если не задан

	// --- НОВОЕ: автогенерация PDF для contract|invoice ---
	switch doc.DocType {
	case "contract", "invoice":
		if s.PDFGen == nil {
			return 0, errors.New("pdf generator not configured")
		}
		// нам нужен title лида — возьмём по deal.LeadID
		lead, lerr := s.LeadRepo.GetByID(deal.LeadID)
		if lerr != nil || lead == nil {
			return 0, errors.New("lead not found")
		}

		var relPath string
		switch doc.DocType {
		case "contract":
			relPath, err = s.PDFGen.GenerateContract(pdf.ContractData{
				LeadTitle: lead.Title,
				DealID:    deal.ID,
				Amount:    deal.Amount,
				Currency:  deal.Currency,
				CreatedAt: deal.CreatedAt,
				Filename:  filename, // если пусто — генератор сам придумает
			})
		case "invoice":
			relPath, err = s.PDFGen.GenerateInvoice(pdf.InvoiceData{
				LeadTitle: lead.Title,
				DealID:    deal.ID,
				Amount:    deal.Amount,
				Currency:  deal.Currency,
				CreatedAt: deal.CreatedAt,
				Filename:  filename,
			})
		}
		if err != nil {
			return 0, err
		}
		doc.FilePath = relPath // вида "/contract_deal_3.pdf"

	default:
		// Если тип не поддержан генератором, но клиент прислал file_path —
		// оставим basename (как было), иначе вернём ошибку.
		if filename == "" {
			return 0, errors.New("unsupported doc_type")
		}
		doc.FilePath = filename
	}

	return s.DocRepo.Create(doc)
}

func (s *DocumentService) GetDocument(id int64, userID, roleID int) (*models.Document, error) {
	doc, err := s.DocRepo.GetByID(id)
	if err != nil || doc == nil {
		return nil, err
	}
	// Sales видит документ только если владеет сделкой
	if roleID == authz.RoleSales {
		deal, derr := s.DealRepo.GetByID(int(doc.DealID))
		if derr != nil || deal == nil {
			return nil, errors.New("not found")
		}
		if deal.OwnerID != userID {
			return nil, errors.New("forbidden")
		}
	}
	return doc, nil
}

func (s *DocumentService) ListDocuments(limit, offset int) ([]*models.Document, error) {
	return s.DocRepo.ListDocuments(limit, offset)
}

func (s *DocumentService) ListDocumentsByDeal(dealID int64, userID, roleID int) ([]*models.Document, error) {
	deal, err := s.DealRepo.GetByID(int(dealID))
	if err != nil || deal == nil {
		return nil, errors.New("not found")
	}
	// Sales — только свои сделки
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return nil, errors.New("forbidden")
	}
	return s.DocRepo.ListDocumentsByDeal(dealID)
}

func (s *DocumentService) DeleteDocument(id int64, userID, roleID int) error {
	if authz.IsReadOnly(roleID) {
		return errors.New("read-only role")
	}
	doc, err := s.DocRepo.GetByID(id)
	if err != nil || doc == nil {
		return errors.New("not found")
	}
	deal, derr := s.DealRepo.GetByID(int(doc.DealID))
	if derr != nil || deal == nil {
		return errors.New("not found")
	}
	// Sales — только свои; Ops/Mgmt/Admin — можно; Audit — запрещено (срезано выше)
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return errors.New("forbidden")
	}
	return s.DocRepo.Delete(id)
}

// ===== Изменение статусов =====
//
// Стандартный поток:
// draft -> under_review -> approved|returned -> signed

func (s *DocumentService) Submit(id int64, userID, roleID int) error {
	if authz.IsReadOnly(roleID) {
		return errors.New("read-only role")
	}
	doc, err := s.DocRepo.GetByID(id)
	if err != nil || doc == nil {
		return errors.New("not found")
	}
	deal, derr := s.DealRepo.GetByID(int(doc.DealID))
	if derr != nil || deal == nil {
		return errors.New("not found")
	}
	// Sales может сабмитить только документы своей сделки
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return errors.New("forbidden")
	}
	if doc.Status != "draft" {
		return errors.New("invalid status")
	}
	return s.DocRepo.UpdateStatus(id, "under_review")
}

func (s *DocumentService) Review(id int64, action string, userID, roleID int) error {
	// Только Ops/Mgmt/Admin
	if !(roleID == authz.RoleOperations || roleID == authz.RoleManagement || roleID == authz.RoleAdmin) {
		return errors.New("forbidden")
	}
	doc, err := s.DocRepo.GetByID(id)
	if err != nil || doc == nil {
		return errors.New("not found")
	}
	if doc.Status != "under_review" {
		return errors.New("invalid status")
	}
	switch action {
	case "approve":
		return s.DocRepo.UpdateStatus(id, "approved")
	case "return":
		return s.DocRepo.UpdateStatus(id, "returned")
	default:
		return errors.New("bad action")
	}
}

func (s *DocumentService) Sign(id int64, userID, roleID int) error {
	// Только Mgmt/Admin вручную
	if !(roleID == authz.RoleManagement || roleID == authz.RoleAdmin) {
		return errors.New("forbidden")
	}
	doc, err := s.DocRepo.GetByID(id)
	if err != nil || doc == nil {
		return errors.New("not found")
	}
	if !(doc.Status == "approved" || doc.Status == "returned") {
		return errors.New("invalid status")
	}
	now := time.Now()
	doc.Status = "signed"
	doc.SignedAt = &now
	return s.DocRepo.Update(doc)
}

// SignBySMS — “механическое” подписание после успешного подтверждения SMS.
// Вызывается из SMSService после валидации кода.
func (s *DocumentService) SignBySMS(docID int64) error {
	doc, err := s.DocRepo.GetByID(docID)
	if err != nil || doc == nil {
		return errors.New("not found")
	}
	// Строже: разрешить только из approved
	// if doc.Status != "approved" { return errors.New("invalid status") }
	// Мягче: разрешить из approved/returned/under_review
	if !(doc.Status == "approved" || doc.Status == "returned" || doc.Status == "under_review") {
		return errors.New("invalid status")
	}
	now := time.Now()
	doc.Status = "signed"
	doc.SignedAt = &now
	return s.DocRepo.Update(doc)
}

// ===== Работа с файлами (RBAC + защита пути) =====

func (s *DocumentService) resolveAndAuthorizeFile(docID int64, userID, roleID int) (absPath, fileName string, err error) {
	doc, err := s.DocRepo.GetByID(docID)
	if err != nil || doc == nil {
		return "", "", errors.New("not found")
	}
	deal, derr := s.DealRepo.GetByID(int(doc.DealID))
	if derr != nil || deal == nil {
		return "", "", errors.New("not found")
	}
	// Sales — только свои документы
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return "", "", errors.New("forbidden")
	}

	// Нормализуем путь: хранится только имя файла (или относительный путь вида "/name.pdf")
	rel := strings.TrimSpace(doc.FilePath)
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.TrimPrefix(rel, "/")
	rel = strings.TrimPrefix(rel, "files/")
	rel = filepath.Base(rel) // безопасность: никакой вложенности

	if rel == "" || rel == "." {
		return "", "", errors.New("bad filepath")
	}

	abs := filepath.Join(s.FilesRoot, rel)
	info, statErr := os.Stat(abs)
	if statErr != nil || info.IsDir() {
		return "", "", errors.New("file not found")
	}
	return abs, filepath.Base(abs), nil
}

// ResolveFileForHTTP — экспортируемый метод для хендлеров (inline/attachment)
func (s *DocumentService) ResolveFileForHTTP(docID int64, userID, roleID int, _ bool) (string, string, error) {
	return s.resolveAndAuthorizeFile(docID, userID, roleID)
}

// ===== Создание документа из лида с автогенерацией PDF =====

func (s *DocumentService) CreateDocumentFromLead(leadID int, docType string, userID, roleID int) (*models.Document, error) {
	lead, err := s.LeadRepo.GetByID(leadID)
	if err != nil || lead == nil {
		return nil, errors.New("lead not found")
	}
	deal, err := s.DealRepo.GetByLeadID(leadID)
	if err != nil || deal == nil {
		return nil, errors.New("deal not found")
	}
	// Sales — только свои
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return nil, errors.New("forbidden")
	}

	// Генерация PDF (поддерживаем contract | invoice; остальное — 400)
	var relPath string
	switch docType {
	case "contract":
		if s.PDFGen == nil {
			return nil, errors.New("pdf generator not configured")
		}
		relPath, err = s.PDFGen.GenerateContract(pdf.ContractData{
			LeadTitle: lead.Title,
			DealID:    deal.ID,
			Amount:    deal.Amount,
			Currency:  deal.Currency,
			CreatedAt: deal.CreatedAt,
			// Filename: можно не указывать — сгенерируется автоматически
		})
		if err != nil {
			return nil, err
		}
	case "invoice":
		if s.PDFGen == nil {
			return nil, errors.New("pdf generator not configured")
		}
		relPath, err = s.PDFGen.GenerateInvoice(pdf.InvoiceData{
			LeadTitle: lead.Title,
			DealID:    deal.ID,
			Amount:    deal.Amount,
			Currency:  deal.Currency,
			CreatedAt: deal.CreatedAt,
		})
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unsupported doc_type")
	}

	doc := &models.Document{
		DealID:   int64(deal.ID),
		DocType:  docType,
		Status:   "draft",
		FilePath: relPath, // например: "/contract_deal_1.pdf"
	}
	id, ierr := s.DocRepo.Create(doc)
	if ierr != nil {
		return nil, ierr
	}
	doc.ID = id
	return doc, nil
}
