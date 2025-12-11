package services

import (
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"turcompany/internal/authz"
	"turcompany/internal/docx"
	"turcompany/internal/models"
	"turcompany/internal/pdf"
	"turcompany/internal/repositories"
	"turcompany/internal/xlsx"
)

type DocumentRepo interface {
	Create(doc *models.Document) (int64, error)
	GetByID(id int64) (*models.Document, error)
	ListDocuments(limit, offset int) ([]*models.Document, error)
	ListDocumentsByDeal(dealID int64) ([]*models.Document, error)
	Delete(id int64) error
	UpdateStatus(id int64, status string) error
}

type LeadRepo interface {
	GetByID(id int) (*models.Leads, error)
}

type DealRepo interface {
	GetByID(id int) (*models.Deals, error)
	GetByLeadID(leadID int) (*models.Deals, error)
	GetLatestByClientID(clientID int) (*models.Deals, error)
}

type ClientRepo interface {
	GetByID(id int) (*models.Client, error)
}

type SMSConfirmRepo interface {
	Create(sms *models.SMSConfirmation) (int64, error)
	GetLatestByDocumentID(documentID int64) (*models.SMSConfirmation, error)
	GetByDocumentIDAndCode(documentID int64, code string) (*models.SMSConfirmation, error)
	Update(sms *models.SMSConfirmation) error
}

type DocumentService struct {
	DocRepo    DocumentRepo
	LeadRepo   LeadRepo
	DealRepo   DealRepo
	ClientRepo ClientRepo
	SMSRepo    SMSConfirmRepo
	SignSecret string

	FilesRoot string        // корень хранения файлов (cfg.Files.RootDir)
	PDFGen    pdf.Generator // генератор PDF (internal/pdf, txt-шаблоны/старые контракты)
	DocxGen   docx.Generator
	XlsxGen   xlsx.Generator
}

var (
	_ DocumentRepo   = (*repositories.DocumentRepository)(nil)
	_ LeadRepo       = (*repositories.LeadRepository)(nil)
	_ DealRepo       = (*repositories.DealRepository)(nil)
	_ ClientRepo     = (*repositories.ClientRepository)(nil)
	_ SMSConfirmRepo = (*repositories.SMSConfirmationRepository)(nil)
)

func NewDocumentService(
	docRepo DocumentRepo,
	leadRepo LeadRepo,
	dealRepo DealRepo,
	clientRepo ClientRepo,
	smsRepo SMSConfirmRepo,
	signSecret string,
	filesRoot string,
	pdfGen pdf.Generator,
	docxGen docx.Generator,
	xlsxGen xlsx.Generator,
) *DocumentService {
	return &DocumentService{
		DocRepo:    docRepo,
		LeadRepo:   leadRepo,
		DealRepo:   dealRepo,
		ClientRepo: clientRepo,
		SMSRepo:    smsRepo,
		SignSecret: signSecret,
		FilesRoot:  filesRoot,
		PDFGen:     pdfGen,
		DocxGen:    docxGen,
		XlsxGen:    xlsxGen,
	}
}

// DOCX-шаблоны для client-ориентированных документов
var clientDocDocxMap = map[string]string{
	"contract_full":          "contract_full.docx",
	"contract_50_50":         "contract_50_50.docx",
	"personal_data_consent":  "personal_data_consent.docx",
	"refund_receipt_full":    "refund_receipt_full.docx",
	"refund_receipt_partial": "refund_receipt_partial.docx",
	"refund_application":     "refund_application.docx",
	"pause_application":      "pause_application.docx",
	"additional_agreement":   "additional_agreement.docx",
}

// TXT-шаблоны (fallback, старый режим)
var clientDocTemplates = map[string]struct {
	FileName  string
	NeedsDeal bool
}{
	"contract": {
		FileName:  "contract_full.txt",
		NeedsDeal: true,
	},
	"contract_full": {
		FileName:  "contract_full.txt",
		NeedsDeal: true,
	},
	"contract_50_50": {
		FileName:  "contract_50_50.txt",
		NeedsDeal: true,
	},
	"personal_data_consent": {
		FileName:  "personal_data_consent.txt",
		NeedsDeal: true,
	},
	"refund_receipt_full": {
		FileName:  "refund_receipt_full.txt",
		NeedsDeal: true,
	},
	"refund_receipt_partial": {
		FileName:  "refund_receipt_partial.txt",
		NeedsDeal: true,
	},
	"refund_application": {
		FileName:  "refund_application.txt",
		NeedsDeal: true,
	},
	"pause_application": {
		FileName:  "pause_application.txt",
		NeedsDeal: true,
	},
	"additional_agreement": {
		FileName:  "additional_agreement.txt",
		NeedsDeal: true,
	},
}

// EXCEL-шаблоны
var clientExcelTemplates = map[string]struct {
	Template  string
	NeedsDeal bool
}{
	"personal_data_excel": {
		Template:  "personal_data.xlsx",
		NeedsDeal: true, // нужен контракт/сделка (CONTRACT_NUMBER)
	},
}

// ====================== CRUD ======================

func (s *DocumentService) CreateDocument(doc *models.Document, userID, roleID int) (int64, error) {
	if authz.IsReadOnly(roleID) {
		return 0, errors.New("read-only role")
	}
	if roleID == authz.RoleAdminStaff {
		return 0, errors.New("forbidden")
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

	// Автогенерация PDF для старых типов contract | invoice
	switch doc.DocType {
	case "contract", "invoice":
		if s.PDFGen == nil {
			return 0, errors.New("pdf generator not configured")
		}
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
				Filename:  filename,
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
		doc.FilePath = relPath
		doc.FilePathPdf = relPath

	default:
		// Если тип не поддержан генератором, но клиент прислал file_path —
		// оставим basename, иначе ошибка.
		if filename == "" {
			return 0, errors.New("unsupported doc_type")
		}
		doc.FilePath = filename
	}

	return s.DocRepo.Create(doc)
}

// UploadDocument сохраняет присланный файл и создает запись документа.
func (s *DocumentService) UploadDocument(dealID int64, docType string, file *multipart.FileHeader, userID, roleID int) (*models.Document, error) {
	if authz.IsReadOnly(roleID) {
		return nil, errors.New("read-only role")
	}
	if dealID == 0 {
		return nil, errors.New("deal not found")
	}
	deal, err := s.DealRepo.GetByID(int(dealID))
	if err != nil || deal == nil {
		return nil, errors.New("deal not found")
	}
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return nil, errors.New("forbidden")
	}
	if strings.TrimSpace(docType) == "" {
		return nil, errors.New("doc_type is required")
	}
	if file == nil {
		return nil, errors.New("file is required")
	}

	safeName := filepath.Base(file.Filename)
	if safeName == "" || safeName == "." {
		return nil, errors.New("invalid filename")
	}
	if err := os.MkdirAll(s.FilesRoot, 0o755); err != nil {
		return nil, fmt.Errorf("prepare files dir: %w", err)
	}
	finalName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), safeName)
	dstPath := filepath.Join(s.FilesRoot, finalName)

	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open upload: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return nil, fmt.Errorf("create file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return nil, fmt.Errorf("save file: %w", err)
	}

	doc := &models.Document{
		DealID:   dealID,
		DocType:  docType,
		FilePath: finalName,
		Status:   "draft",
	}
	id, err := s.DocRepo.Create(doc)
	if err != nil {
		return nil, err
	}
	doc.ID = id
	return doc, nil
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
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return errors.New("forbidden")
	}
	return s.DocRepo.Delete(id)
}

// ================== Статусы ==================

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
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return errors.New("forbidden")
	}
	if doc.Status != "draft" {
		return errors.New("invalid status")
	}
	return s.DocRepo.UpdateStatus(id, "under_review")
}

func (s *DocumentService) Review(id int64, action string, userID, roleID int) error {
	if !(roleID == authz.RoleOperations || roleID == authz.RoleManagement) {
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
	// Только Management вручную
	if roleID != authz.RoleManagement {
		return errors.New("forbidden")
	}
	doc, err := s.DocRepo.GetByID(id)
	if err != nil || doc == nil {
		return errors.New("not found")
	}
	if !(doc.Status == "approved" || doc.Status == "returned") {
		return errors.New("invalid status")
	}
	// Просто меняем статус на signed
	return s.DocRepo.UpdateStatus(id, "signed")
}

// SignBySMS — подписание по SMS (автоматическое)
func (s *DocumentService) SignBySMS(docID int64) error {
	doc, err := s.DocRepo.GetByID(docID)
	if err != nil || doc == nil {
		return errors.New("not found")
	}
	// Разрешаем из approved/returned/under_review
	if !(doc.Status == "approved" || doc.Status == "returned" || doc.Status == "under_review") {
		return errors.New("invalid status")
	}
	return s.DocRepo.UpdateStatus(docID, "signed")
}

// ================== Работа с файлами ==================

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

	// Теперь FilePath может быть "/pdf/xxx.pdf" или "xxx.pdf" (для upload)
	rel := strings.TrimSpace(doc.FilePath)
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.TrimPrefix(rel, "/")

	// если кто-то вдруг сохранил "files/..." — убираем префикс
	if strings.HasPrefix(rel, "files/") {
		rel = strings.TrimPrefix(rel, "files/")
	}

	// защита от ".."
	if strings.Contains(rel, "..") {
		return "", "", errors.New("bad filepath")
	}

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

func (s *DocumentService) ResolveFileForHTTP(docID int64, userID, roleID int, _ bool) (string, string, error) {
	return s.resolveAndAuthorizeFile(docID, userID, roleID)
}

// ================== Документы из лида (старый контракт/invoice) ==================

func (s *DocumentService) CreateDocumentFromLead(leadID int, docType string, userID, roleID int) (*models.Document, error) {
	lead, err := s.LeadRepo.GetByID(leadID)
	if err != nil || lead == nil {
		return nil, errors.New("lead not found")
	}
	deal, err := s.DealRepo.GetByLeadID(leadID)
	if err != nil || deal == nil {
		return nil, errors.New("deal not found")
	}
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return nil, errors.New("forbidden")
	}

	if s.PDFGen == nil {
		return nil, errors.New("pdf generator not configured")
	}

	var relPath string
	switch docType {
	case "contract":
		relPath, err = s.PDFGen.GenerateContract(pdf.ContractData{
			LeadTitle: lead.Title,
			DealID:    deal.ID,
			Amount:    deal.Amount,
			Currency:  deal.Currency,
			CreatedAt: deal.CreatedAt,
		})
	case "invoice":
		relPath, err = s.PDFGen.GenerateInvoice(pdf.InvoiceData{
			LeadTitle: lead.Title,
			DealID:    deal.ID,
			Amount:    deal.Amount,
			Currency:  deal.Currency,
			CreatedAt: deal.CreatedAt,
		})
	default:
		return nil, errors.New("unsupported doc_type")
	}
	if err != nil {
		return nil, err
	}

	doc := &models.Document{
		DealID:      int64(deal.ID),
		DocType:     docType,
		Status:      "draft",
		FilePath:    relPath,
		FilePathPdf: relPath,
	}
	id, ierr := s.DocRepo.Create(doc)
	if ierr != nil {
		return nil, ierr
	}
	doc.ID = id
	return doc, nil
}

// ================== Документы из клиента (новый поток) ==================

func (s *DocumentService) CreateDocumentFromClient(
	clientID int,
	dealID int,
	docType string,
	userID, roleID int,
	extra map[string]string,
) (*models.Document, error) {

	// хотя бы один генератор должен быть сконфигурирован
	if s.PDFGen == nil && s.DocxGen == nil && s.XlsxGen == nil {
		return nil, errors.New("no document generator configured")
	}

	// какие типы шаблонов у нас есть для этого docType
	txtCfg, hasTxt := clientDocTemplates[docType]
	docxName, hasDocx := clientDocDocxMap[docType]
	excelCfg, hasExcel := clientExcelTemplates[docType]

	if !hasTxt && !hasDocx && !hasExcel {
		return nil, errors.New("unsupported doc_type")
	}

	// --- Клиент ---
	client, err := s.ClientRepo.GetByID(clientID)
	if err != nil || client == nil {
		return nil, errors.New("client not found")
	}

	// --- Нужна ли сделка? ---
	needDeal := false
	if hasTxt && txtCfg.NeedsDeal {
		needDeal = true
	}
	if hasDocx {
		needDeal = true
	}
	if hasExcel && excelCfg.NeedsDeal {
		needDeal = true
	}

	var deal *models.Deals
	if needDeal {
		if dealID > 0 {
			deal, err = s.DealRepo.GetByID(dealID)
			if err != nil || deal == nil {
				return nil, errors.New("deal not found")
			}
		} else {
			deal, err = s.DealRepo.GetLatestByClientID(clientID)
			if err != nil {
				return nil, err
			}
			if deal == nil {
				return nil, errors.New("deal not found")
			}
		}

		if roleID == authz.RoleSales && deal.OwnerID != userID {
			return nil, errors.New("forbidden")
		}
	}

	now := time.Now()

	// базовые плейсхолдеры
	placeholders := buildClientPlaceholders(client, deal, extra, now)

	// спец-логика для чекбоксов
	switch docType {
	case "refund_application":
		code := extra["REFUND_REASON_CODE"]
		flags := map[string]string{
			"RA_R1": " ",
			"RA_R2": " ",
			"RA_R3": " ",
			"RA_R4": " ",
			"RA_R5": " ",
			"RA_R6": " ",
		}
		switch code {
		case "visa_refusal":
			flags["RA_R1"] = "X"
		case "interview_fail":
			flags["RA_R2"] = "X"
		case "family":
			flags["RA_R3"] = "X"
		case "health":
			flags["RA_R4"] = "X"
		case "force_majeure":
			flags["RA_R5"] = "X"
		case "other":
			flags["RA_R6"] = "X"
		}
		for k, v := range flags {
			placeholders[k] = v
		}

	case "pause_application":
		code := extra["PAUSE_REASON_CODE"]
		flags := map[string]string{
			"PA_R1": " ",
			"PA_R2": " ",
			"PA_R3": " ",
			"PA_R4": " ",
		}
		switch code {
		case "health":
			flags["PA_R1"] = "X"
		case "force_majeure":
			flags["PA_R2"] = "X"
		case "family":
			flags["PA_R3"] = "X"
		case "other":
			flags["PA_R4"] = "X"
		}
		for k, v := range flags {
			placeholders[k] = v
		}
	}

	// базовое имя файла без расширения
	baseFilename := fmt.Sprintf(
		"%s_client_%d_%s",
		docType,
		client.ID,
		now.Format("20060102_150405"),
	)

	// ================== EXCEL ==================
	if hasExcel {
		if s.XlsxGen == nil {
			return nil, errors.New("xlsx generator not configured")
		}

		excelRelPath, err := s.XlsxGen.GenerateFromTemplate(
			excelCfg.Template,
			placeholders,
			baseFilename,
		)
		if err != nil {
			return nil, wrapGenerationError(docType, err)
		}

		excelRelPath = normalizeStoragePath(excelRelPath)

		doc := &models.Document{
			DealID:   getDealID64(deal),
			DocType:  docType,
			Status:   "draft",
			FilePath: excelRelPath, // "/excel/..."
		}

		id, err := s.DocRepo.Create(doc)
		if err != nil {
			return nil, err
		}
		doc.ID = id
		return doc, nil
	}

	// ================== DOCX ==================
	if hasDocx && s.DocxGen != nil {
		docxRelPath, pdfRelPath, err := s.DocxGen.GenerateDocxAndPDF(
			docxName,
			placeholders,
			baseFilename,
		)
		if err != nil {
			return nil, wrapGenerationError(docType, err)
		}

		docxRelPath = normalizeStoragePath(docxRelPath)
		pdfRelPath = normalizeStoragePath(pdfRelPath)
		mainPath := pdfRelPath
		if mainPath == "" {
			mainPath = docxRelPath
		}

		doc := &models.Document{
			DealID:       getDealID64(deal),
			DocType:      docType,
			Status:       "draft",
			FilePath:     mainPath, // основной путь — PDF если он есть
			FilePathPdf:  pdfRelPath,
			FilePathDocx: docxRelPath,
		}

		id, err := s.DocRepo.Create(doc)
		if err != nil {
			return nil, err
		}
		doc.ID = id
		return doc, nil
	}

	// ================== TXT → PDF ==================
	if !hasTxt {
		return nil, errors.New("txt template not configured")
	}
	if s.PDFGen == nil {
		return nil, errors.New("pdf generator not configured for txt templates")
	}

	pdfRelPath, err := s.PDFGen.GenerateFromTemplate(
		txtCfg.FileName,
		placeholders,
		baseFilename+".pdf",
	)
	if err != nil {
		return nil, wrapGenerationError(docType, err)
	}

	pdfRelPath = normalizeStoragePath(pdfRelPath)

	doc := &models.Document{
		DealID:      getDealID64(deal),
		DocType:     docType,
		Status:      "draft",
		FilePath:    pdfRelPath,
		FilePathPdf: pdfRelPath,
	}

	id, err := s.DocRepo.Create(doc)
	if err != nil {
		return nil, err
	}
	doc.ID = id
	return doc, nil
}

// ================== helpers ==================

func normalizeStoragePath(rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return ""
	}
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.TrimPrefix(rel, "files/")
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return ""
	}
	return "/" + rel
}

func wrapGenerationError(docType string, err error) error {
	log.Printf("[documents] generation failed for %s: %v", docType, err)
	return errors.New("document generation failed")
}

func getDealID64(deal *models.Deals) int64 {
	if deal == nil {
		return 0
	}
	return int64(deal.ID)
}
