package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jung-kurt/gofpdf"

	"turcompany/internal/authz"
	"turcompany/internal/docx"
	"turcompany/internal/models"
	"turcompany/internal/pdf"
	"turcompany/internal/repositories"
	"turcompany/internal/xlsx"
)

var (
	ErrPDFCPUMissing           = errors.New("pdfcpu binary is not available")
	ErrDocumentChangedAfterOTP = errors.New("DOCUMENT_CHANGED_AFTER_OTP")
	logTTFFontsOnce            sync.Once
)

type DocumentRepo interface {
	Create(doc *models.Document) (int64, error)
	GetByID(id int64) (*models.Document, error)
	ListDocuments(limit, offset int) ([]*models.Document, error)
	ListDocumentsByDeal(dealID int64) ([]*models.Document, error)
	Delete(id int64) error
	UpdateStatus(id int64, status string) error
	Update(doc *models.Document) error
	UpdateSigningMeta(id int64, signMethod, signIP, signUserAgent, signMetadata string) error
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

type DocumentService struct {
	DocRepo    DocumentRepo
	LeadRepo   LeadRepo
	DealRepo   DealRepo
	ClientRepo ClientRepo
	SignSecret string

	FilesRoot string        // корень хранения файлов (cfg.Files.RootDir)
	PDFGen    pdf.Generator // генератор PDF (internal/pdf, txt-шаблоны/старые контракты)
	DocxGen   docx.Generator
	XlsxGen   xlsx.Generator
}

var (
	_             DocumentRepo = (*repositories.DocumentRepository)(nil)
	_             LeadRepo     = (*repositories.LeadRepository)(nil)
	_             DealRepo     = (*repositories.DealRepository)(nil)
	_             ClientRepo   = (*repositories.ClientRepository)(nil)
	mergePDFsFunc              = mergePDFs
)

func NewDocumentService(
	docRepo DocumentRepo,
	leadRepo LeadRepo,
	dealRepo DealRepo,
	clientRepo ClientRepo,
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

var supportedDocTypes = func() map[string]struct{} {
	types := map[string]struct{}{
		"contract": {},
		"invoice":  {},
	}
	for key := range clientDocTemplates {
		types[key] = struct{}{}
	}
	for key := range clientDocDocxMap {
		types[key] = struct{}{}
	}
	for key := range clientExcelTemplates {
		types[key] = struct{}{}
	}
	return types
}()

func normalizeDocType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isSupportedDocType(value string) bool {
	_, ok := supportedDocTypes[normalizeDocType(value)]
	return ok
}

// PrepareForSignature подготавливает документ к юридически значимой подписи
func (s *DocumentService) PrepareForSignature(id int64, userID, roleID int) error {
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

	if doc.Status != "approved" {
		return errors.New("document must be approved before signature")
	}

	// Меняем статус на "готов к подписи"
	return s.DocRepo.UpdateStatus(id, "sent_for_signature")
}

// GetSignatureMetadata получает метаданные подписи документа
func (s *DocumentService) GetSignatureMetadata(id int64, userID, roleID int) (map[string]interface{}, error) {
	doc, err := s.DocRepo.GetByID(id)
	if err != nil || doc == nil {
		return nil, errors.New("not found")
	}

	deal, derr := s.DealRepo.GetByID(int(doc.DealID))
	if derr != nil || deal == nil {
		return nil, errors.New("not found")
	}

	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return nil, errors.New("forbidden")
	}

	result := map[string]interface{}{
		"document_id": doc.ID,
		"status":      doc.Status,
		"signed_at":   doc.SignedAt,
		"sign_method": doc.SignMethod,
	}

	// Если есть метаданные подписи
	if doc.SignMetadata != "" {
		result["has_metadata"] = true
		result["sign_ip"] = doc.SignIP
		result["sign_user_agent"] = doc.SignUserAgent
	} else {
		result["has_metadata"] = false
	}

	return result, nil
}

// GetLegalConsentText возвращает юридический текст для подписи
func (s *DocumentService) GetLegalConsentText(docType string) string {
	docNames := map[string]string{
		"contract":              "Договор оказания услуг",
		"contract_full":         "Договор оказания услуг (полная оплата)",
		"contract_50_50":        "Договор оказания услуг (50/50)",
		"personal_data_consent": "Согласие на обработку персональных данных",
		"refund_application":    "Заявление на возврат средств",
		"pause_application":     "Заявление на приостановку услуг",
		"additional_agreement":  "Дополнительное соглашение",
	}

	docName := docNames[docType]
	if docName == "" {
		docName = "Документ"
	}

	return fmt.Sprintf(
		"Настоящим я подтверждаю, что ознакомлен(а) с содержанием документа «%s» и соглашаюсь со всеми его условиями.\n\n"+
			"Я понимаю, что:\n"+
			"1. Данный документ имеет юридическую силу\n"+
			"2. Подписание документа означает мое согласие с изложенными условиями\n"+
			"3. Подпись осуществляется с использованием одноразового кода подтверждения\n"+
			"4. Факт ввода кода считается аналогом собственноручной подписи\n"+
			"5. Я несу ответственность за последствия подписания документа",
		docName,
	)
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

	doc.DocType = normalizeDocType(doc.DocType)
	if !isSupportedDocType(doc.DocType) {
		return 0, errors.New("unsupported doc_type")
	}

	// Sales может создавать документ только по своей сделке
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return 0, errors.New("forbidden")
	}

	// статус по умолчанию
	if strings.TrimSpace(doc.Status) == "" {
		doc.Status = "draft"
	}

	// просто нормализуем путь, НИЧЕГО не генерируем
	doc.FilePath = filepath.Base(strings.TrimSpace(doc.FilePath))

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
	docType = normalizeDocType(docType)
	if docType == "" {
		return nil, errors.New("doc_type is required")
	}
	if !isSupportedDocType(docType) {
		return nil, errors.New("unsupported doc_type")
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

func (s *DocumentService) ResolveSignerEmail(id int64, userID, roleID int, fallbackEmail string) (string, error) {
	doc, err := s.GetDocument(id, userID, roleID)
	if err != nil || doc == nil {
		return "", err
	}
	fallbackEmail = strings.TrimSpace(fallbackEmail)
	if s.DealRepo == nil || s.ClientRepo == nil {
		if fallbackEmail == "" {
			return "", errors.New("signer email is required")
		}
		return fallbackEmail, nil
	}
	deal, err := s.DealRepo.GetByID(int(doc.DealID))
	if err != nil || deal == nil {
		if fallbackEmail == "" {
			return "", errors.New("signer email is required")
		}
		return fallbackEmail, nil
	}
	client, err := s.ClientRepo.GetByID(deal.ClientID)
	if err != nil || client == nil {
		if fallbackEmail == "" {
			return "", errors.New("signer email is required")
		}
		return fallbackEmail, nil
	}
	if email := strings.TrimSpace(client.Email); email != "" {
		return email, nil
	}
	if fallbackEmail == "" {
		return "", errors.New("signer email is required")
	}
	return fallbackEmail, nil
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
	if err := s.deleteDocumentFiles(doc); err != nil {
		return err
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

func (s *DocumentService) FinalizeSigning(docID int64) error {
	doc, err := s.DocRepo.GetByID(docID)
	if err != nil || doc == nil {
		return errors.New("not found")
	}

	// уже подписан — просто выходим
	if doc.Status == "signed" {
		return nil
	}

	if doc.Status != "approved" {
		return errors.New("invalid status")
	}

	return s.DocRepo.UpdateStatus(docID, "signed")
}

func (s *DocumentService) FinalizeSignedArtifact(session *models.SignSession) error {
	if session == nil {
		return errors.New("sign session is nil")
	}
	doc, err := s.DocRepo.GetByID(session.DocumentID)
	if err != nil || doc == nil {
		return errors.New("not found")
	}
	originalRel := strings.TrimSpace(doc.FilePathPdf)
	if originalRel == "" {
		originalRel = strings.TrimSpace(doc.FilePath)
	}
	originalAbs, err := s.resolveStoragePath(originalRel)
	if err != nil || originalAbs == "" {
		return errors.New("bad filepath")
	}
	if strings.ToLower(filepath.Ext(originalAbs)) != ".pdf" {
		return errors.New("file not found")
	}
	if _, err := os.Stat(originalAbs); err != nil {
		return errors.New("file not found")
	}
	if err := s.validateSessionDocumentHash(originalAbs, session.DocHash); err != nil {
		return err
	}

	if doc.Status == "signed" {
		if signedRel := extractSignedPDFPath(doc.SignMetadata); signedRel != "" {
			if signedAbs, err := s.resolveStoragePath(signedRel); err == nil && signedAbs != "" {
				if info, statErr := os.Stat(signedAbs); statErr == nil && !info.IsDir() {
					return nil
				}
			}
		}
	}

	base := strings.TrimSuffix(filepath.Base(originalAbs), filepath.Ext(originalAbs))
	signedName := base + "_signed.pdf"
	signedAbs := filepath.Join(s.FilesRoot, "pdf", signedName)
	signedRel := "/pdf/" + signedName

	signPageAbs := filepath.Join(s.FilesRoot, "pdf", fmt.Sprintf("%s_sign_page_%d.pdf", base, time.Now().UnixNano()))
	if err := s.buildSigningPagePDF(doc, session, signPageAbs); err != nil {
		return err
	}
	defer os.Remove(signPageAbs)

	if err := mergePDFsFunc(originalAbs, signPageAbs, signedAbs); err != nil {
		return err
	}

	meta := map[string]any{"signed_pdf_path": signedRel}
	metaRaw, _ := json.Marshal(meta)
	if err := s.DocRepo.UpdateSigningMeta(doc.ID, "email_otp", session.SignedIP, session.SignedUserAgent, string(metaRaw)); err != nil {
		return err
	}
	return nil
}

func (s *DocumentService) validateSessionDocumentHash(path string, expected string) error {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return nil
	}
	current, err := sha256File(path)
	if err != nil {
		return fmt.Errorf("hash document file: %w", err)
	}
	if !strings.EqualFold(current, expected) {
		return ErrDocumentChangedAfterOTP
	}
	return nil
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

func (s *DocumentService) EnsureSigningAllowed(docID int64, userID, roleID int) error {
	doc, err := s.DocRepo.GetByID(docID)
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
	if doc.Status != "approved" {
		return errors.New("invalid status")
	}
	return nil
}

// variant:
//   - "main"     : всегда doc.FilePath
//   - "pdf"      : doc.FilePathPdf если есть, иначе doc.FilePath
//   - "docx"     : doc.FilePathDocx
//   - "xlsx"     : doc.FilePath (только если .xlsx)
//   - "original" : предпочесть docx если есть, иначе doc.FilePath
func (s *DocumentService) ResolveFileForHTTP(docID int64, userID, roleID int, variant string) (string, string, error) {
	doc, err := s.DocRepo.GetByID(docID)
	if err != nil || doc == nil {
		return "", "", errors.New("not found")
	}
	deal, derr := s.DealRepo.GetByID(int(doc.DealID))
	if derr != nil || deal == nil {
		return "", "", errors.New("not found")
	}
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return "", "", errors.New("forbidden")
	}

	variant = strings.ToLower(strings.TrimSpace(variant))
	var rel string
	switch variant {
	case "", "main":
		rel = doc.FilePath
	case "pdf":
		if doc.Status == "signed" {
			if signedPath := extractSignedPDFPath(doc.SignMetadata); signedPath != "" {
				rel = signedPath
			}
		}
		if strings.TrimSpace(rel) == "" && strings.TrimSpace(doc.FilePathPdf) != "" {
			rel = doc.FilePathPdf
		} else {
			if strings.TrimSpace(rel) == "" {
				rel = doc.FilePath
			}
		}
	case "docx":
		rel = doc.FilePathDocx
		if strings.TrimSpace(rel) == "" {
			return "", "", errors.New("file not found")
		}
	case "xlsx":
		rel = doc.FilePath
		if strings.ToLower(filepath.Ext(strings.TrimSpace(rel))) != ".xlsx" {
			return "", "", errors.New("file not found")
		}
	case "original", "source":
		if strings.TrimSpace(doc.FilePathDocx) != "" {
			rel = doc.FilePathDocx
		} else {
			rel = doc.FilePath
		}
	default:
		rel = doc.FilePath
	}

	// normalize + validate
	rel = strings.TrimSpace(rel)
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.TrimPrefix(rel, "/")
	if strings.HasPrefix(rel, "files/") {
		rel = strings.TrimPrefix(rel, "files/")
	}
	if strings.Contains(rel, "..") || rel == "" || rel == "." {
		return "", "", errors.New("bad filepath")
	}

	abs := filepath.Join(s.FilesRoot, rel)
	info, statErr := os.Stat(abs)
	if statErr != nil || info.IsDir() {
		return "", "", errors.New("file not found")
	}
	return abs, filepath.Base(abs), nil
}

func (s *DocumentService) resolveStoragePath(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", nil
	}
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.TrimPrefix(rel, "/")
	if strings.HasPrefix(rel, "files/") {
		rel = strings.TrimPrefix(rel, "files/")
	}
	if strings.Contains(rel, "..") || rel == "." || rel == "" {
		return "", errors.New("bad filepath")
	}
	return filepath.Join(s.FilesRoot, rel), nil
}

func (s *DocumentService) deleteDocumentFiles(doc *models.Document) error {
	paths := []string{
		doc.FilePath,
		doc.FilePathPdf,
		doc.FilePathDocx,
	}
	unique := make(map[string]struct{}, len(paths))
	for _, rel := range paths {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}
		unique[rel] = struct{}{}
	}
	for rel := range unique {
		abs, err := s.resolveStoragePath(rel)
		if err != nil {
			return err
		}
		if abs == "" {
			continue
		}
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("file delete failed: %w", err)
		}
	}
	return nil
}

// ================== Документы из лида (старый контракт/invoice) ==================

func (s *DocumentService) CreateDocumentFromLead(leadID int, docType string, userID, roleID int) (*models.Document, error) {
	docType = normalizeDocType(docType)
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

	amountStr := strconv.FormatFloat(deal.Amount, 'f', 2, 64)
	var relPath string
	switch docType {
	case "contract":
		relPath, err = s.PDFGen.GenerateContract(pdf.ContractData{
			LeadTitle: lead.Title,
			DealID:    deal.ID,
			Amount:    amountStr,
			Currency:  deal.Currency,
			CreatedAt: deal.CreatedAt,
		})
	case "invoice":
		relPath, err = s.PDFGen.GenerateInvoice(pdf.InvoiceData{
			LeadTitle: lead.Title,
			DealID:    deal.ID,
			Amount:    amountStr,
			Currency:  deal.Currency,
			CreatedAt: deal.CreatedAt,
		})
	default:
		return nil, errors.New("unsupported doc_type")
	}
	if err != nil {
		return nil, err
	}

	relPath = normalizeStoragePath(relPath)

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
	docType = normalizeDocType(docType)

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

	missing := validateClientFieldsForDocType(docType, client)
	if len(missing) > 0 {
		return nil, &MissingFieldsError{Fields: missing}
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

func validateClientFieldsForDocType(docType string, c *models.Client) (missing []string) {
	if c == nil {
		return []string{"last_name", "first_name", "iin", "bin_iin", "address", "phone"}
	}

	requireCommon := false
	requireConsent := false
	switch docType {
	case "contract_full", "contract_50_50", "additional_agreement", "refund_receipt_full", "refund_receipt_partial", "refund_application", "pause_application", "personal_data_excel":
		requireCommon = true
	case "personal_data_consent":
		requireConsent = true
	default:
		return nil
	}

	add := func(field string) {
		for _, ex := range missing {
			if ex == field {
				return
			}
		}
		missing = append(missing, field)
	}

	hasName := strings.TrimSpace(c.Name) != "" || (strings.TrimSpace(c.LastName) != "" && strings.TrimSpace(c.FirstName) != "")
	if !hasName {
		add("last_name")
		add("first_name")
	}
	if strings.TrimSpace(c.IIN) == "" && strings.TrimSpace(c.BinIin) == "" {
		add("iin")
		add("bin_iin")
	}
	if strings.TrimSpace(c.Address) == "" && strings.TrimSpace(c.ActualAddress) == "" && strings.TrimSpace(c.RegistrationAddress) == "" {
		add("address")
	}
	if requireCommon && strings.TrimSpace(c.Phone) == "" {
		add("phone")
	}
	if requireConsent {
		if strings.TrimSpace(c.IDNumber) == "" {
			add("id_number")
		}
		if strings.TrimSpace(c.PassportNumber) == "" {
			add("passport_number")
		}
	}
	return missing
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

func extractSignedPDFPath(metaRaw string) string {
	metaRaw = strings.TrimSpace(metaRaw)
	if metaRaw == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(metaRaw), &payload); err != nil {
		return ""
	}
	value, _ := payload["signed_pdf_path"].(string)
	return strings.TrimSpace(value)
}

func (s *DocumentService) buildSigningPagePDF(doc *models.Document, session *models.SignSession, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create sign page dir: %w", err)
	}
	fontPath, err := resolveSigningFontPath()
	if err != nil {
		return err
	}
	fontPath, err = ensureSigningFontInTemp(fontPath)
	if err != nil {
		return err
	}

	pdfFile := gofpdf.New("P", "mm", "A4", "/tmp")
	pdfFile.SetCompression(false)
	pdfFile.SetTitle("Лист подписания", false)
	pdfFile.SetAuthor("KUB CRM", false)
	pdfFile.AddUTF8Font("dejavu", "", filepath.Base(fontPath))
	if err := pdfFile.Error(); err != nil {
		return fmt.Errorf("register UTF-8 font dejavu regular: %w", err)
	}
	pdfFile.AddUTF8Font("dejavu", "B", filepath.Base(fontPath))
	if err := pdfFile.Error(); err != nil {
		return fmt.Errorf("register UTF-8 font dejavu bold: %w", err)
	}
	pdfFile.SetMargins(15, 15, 15)
	pdfFile.AddPage()
	pdfFile.SetAutoPageBreak(true, 15)
	pdfFile.SetFont("dejavu", "B", 16)
	if err := pdfFile.Error(); err != nil {
		return fmt.Errorf("set signing page font (bold title): %w", err)
	}
	pdfFile.CellFormat(0, 8, "Лист подписания", "", 1, "C", false, 0, "")
	pdfFile.SetTextColor(120, 120, 120)
	pdfFile.SetFont("dejavu", "", 10)
	if err := pdfFile.Error(); err != nil {
		return fmt.Errorf("set signing page font (subtitle): %w", err)
	}
	pdfFile.CellFormat(0, 6, "KUB CRM • электронное подтверждение (ПЭП)", "", 1, "C", false, 0, "")
	pdfFile.SetTextColor(0, 0, 0)
	pdfFile.Ln(2)

	lineY := pdfFile.GetY()
	pdfFile.Line(15, lineY, 195, lineY)
	pdfFile.Ln(4)

	signedAt := ""
	if session.SignedAt != nil {
		loc, _ := time.LoadLocation("Asia/Aqtobe")
		if loc == nil {
			loc = time.UTC
		}
		tzLabel := loc.String()
		if tzLabel == "" || tzLabel == "UTC" {
			tzLabel = session.SignedAt.In(loc).Format("-07:00")
		}
		signedAt = fmt.Sprintf("%s (%s)", session.SignedAt.In(loc).Format("02.01.2006 15:04"), tzLabel)
	}

	docTypeTitle := doc.DocType
	if pretty := map[string]string{
		"contract":              "Договор оказания услуг",
		"contract_full":         "Договор оказания услуг (полная оплата)",
		"contract_50_50":        "Договор оказания услуг (50/50)",
		"personal_data_consent": "Согласие на обработку персональных данных",
		"refund_application":    "Заявление на возврат средств",
		"pause_application":     "Заявление на приостановку услуг",
		"additional_agreement":  "Дополнительное соглашение",
	}[doc.DocType]; pretty != "" {
		docTypeTitle = fmt.Sprintf("%s (%s)", pretty, doc.DocType)
	}

	drawSectionTitle(pdfFile, "Документ")
	drawKeyValue(pdfFile, "Номер/ID", strconv.FormatInt(doc.ID, 10), 45, 5.5)
	drawKeyValue(pdfFile, "Тип", docTypeTitle, 45, 5.5)
	drawKeyValue(pdfFile, "Дата подписания", signedAt, 45, 5.5)

	drawSectionTitle(pdfFile, "Подписант")
	drawKeyValue(pdfFile, "Email", session.SignerEmail, 45, 5.5)
	drawKeyValue(pdfFile, "Метод", "Подписание по коду из письма", 45, 5.5)

	drawSectionTitle(pdfFile, "Технические данные")
	pdfFile.SetFont("dejavu", "", 9)
	drawKeyValue(pdfFile, "IP", session.SignedIP, 45, 5)
	userAgent := strings.Join(wrapText(session.SignedUserAgent, 85), "\n")
	drawKeyValue(pdfFile, "User-Agent", userAgent, 45, 5)

	drawSectionTitle(pdfFile, "Контроль целостности")
	pdfFile.SetFont("dejavu", "", 9)
	hashLine1, hashLine2, hashOK := splitSHA256(session.DocHash)
	if hashOK {
		drawKeyValue(pdfFile, "Хэш документа (SHA-256)", hashLine1, 45, 5)
		drawValueContinuation(pdfFile, hashLine2, 45, 5)
	} else {
		drawKeyValue(pdfFile, "Хэш документа (SHA-256)", "—", 45, 5)
	}

	verifyURL := extractVerifyURL(doc.SignMetadata)
	if verifyURL != "" && !strings.EqualFold(verifyURL, "N/A") {
		drawKeyValue(pdfFile, "Проверка", verifyURL, 45, 5.5)
	}

	pdfFile.Ln(3)
	lineY = pdfFile.GetY()
	pdfFile.Line(15, lineY, 195, lineY)
	pdfFile.Ln(3)
	pdfFile.SetTextColor(90, 90, 90)
	pdfFile.SetFont("dejavu", "", 9)
	if err := pdfFile.Error(); err != nil {
		return fmt.Errorf("set signing page font (note): %w", err)
	}
	pdfFile.MultiCell(0, 4.8, "Подписание выполнено с подтверждением одноразовым кодом, направленным на email подписанта.", "", "L", false)
	pdfFile.SetTextColor(0, 0, 0)

	// Диагностика результата: `pdffonts signed.pdf` — должен показывать embedded DejaVu для страницы подписания.
	if err := pdfFile.OutputFileAndClose(outPath); err != nil {
		return fmt.Errorf("create signing page: %w", err)
	}
	return nil
}

func drawSectionTitle(pdfFile *gofpdf.Fpdf, title string) {
	pdfFile.Ln(1)
	lineY := pdfFile.GetY()
	pdfFile.Line(15, lineY, 195, lineY)
	pdfFile.Ln(2)
	pdfFile.SetFont("dejavu", "B", 11)
	pdfFile.CellFormat(0, 6, title, "", 1, "L", false, 0, "")
}

func drawKeyValue(pdfFile *gofpdf.Fpdf, key, value string, keyWidth, lineHeight float64) {
	pdfFile.SetFont("dejavu", "B", 10)
	_, y := pdfFile.GetXY()
	pdfFile.CellFormat(keyWidth, lineHeight, key+":", "", 0, "L", false, 0, "")
	pdfFile.SetFont("dejavu", "", 10)
	pdfFile.MultiCell(0, lineHeight, value, "", "L", false)
	if pdfFile.GetY() == y {
		pdfFile.Ln(lineHeight)
	}
}

func drawValueContinuation(pdfFile *gofpdf.Fpdf, value string, keyWidth, lineHeight float64) {
	if strings.TrimSpace(value) == "" {
		return
	}
	leftMargin, _, rightMargin, _ := pdfFile.GetMargins()
	pageWidth, _ := pdfFile.GetPageSize()
	valueWidth := pageWidth - rightMargin - (leftMargin + keyWidth)
	pdfFile.SetX(leftMargin + keyWidth)
	pdfFile.MultiCell(valueWidth, lineHeight, value, "", "L", false)
}

func wrapText(text string, maxChars int) []string {
	if maxChars <= 0 || len(text) <= maxChars {
		if strings.TrimSpace(text) == "" {
			return []string{"-"}
		}
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{"-"}
	}
	lines := make([]string, 0, len(words))
	current := words[0]
	for _, word := range words[1:] {
		if len([]rune(current))+1+len([]rune(word)) <= maxChars {
			current += " " + word
			continue
		}
		lines = append(lines, current)
		current = word
	}
	lines = append(lines, current)
	return lines
}

func splitSHA256(hash string) (line1, line2 string, ok bool) {
	hash = strings.TrimSpace(hash)
	if len(hash) != 64 {
		return "", "", false
	}
	for _, r := range hash {
		if ('0' <= r && r <= '9') || ('a' <= r && r <= 'f') || ('A' <= r && r <= 'F') {
			continue
		}
		return "", "", false
	}
	return hash[:32], hash[32:64], true
}

func extractVerifyURL(metaRaw string) string {
	metaRaw = strings.TrimSpace(metaRaw)
	if metaRaw == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(metaRaw), &payload); err != nil {
		return ""
	}
	verifyURL, _ := payload["verify_url"].(string)
	return strings.TrimSpace(verifyURL)
}

func ensureSigningFontInTemp(srcPath string) (string, error) {
	tempPath := filepath.Join(os.TempDir(), filepath.Base(srcPath))
	if srcPath == tempPath {
		return tempPath, nil
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", fmt.Errorf("read sign page font %q: %w", srcPath, err)
	}
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return "", fmt.Errorf("copy sign page font to temp %q: %w", tempPath, err)
	}
	return tempPath, nil
}

func resolveSigningFontPath() (string, error) {
	candidates := []string{
		"/opt/turcompany/assets/fonts/DejaVuSans.ttf",
		"assets/fonts/DejaVuSans.ttf",
		"../../assets/fonts/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/noto/NotoSans-Regular.ttf",
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "assets/fonts/DejaVuSans.ttf"),
			filepath.Join(wd, "../../assets/fonts/DejaVuSans.ttf"),
		)
	}

	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			abs, absErr := filepath.Abs(candidate)
			if absErr == nil {
				return abs, nil
			}
			return candidate, nil
		}
	}

	logTTFFontsOnce.Do(func() {
		fonts := discoverTTFFonts([]string{"/opt/turcompany/assets/fonts", "/usr/share/fonts"})
		log.Printf("[documents] no signing font found in known paths, discovered TTF fonts: %v", fonts)
	})

	return "", fmt.Errorf("sign page font not found: expected one of %v", candidates)
}

func discoverTTFFonts(baseDirs []string) []string {
	fonts := make([]string, 0, 16)
	for _, dir := range baseDirs {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if strings.EqualFold(filepath.Ext(path), ".ttf") {
				fonts = append(fonts, path)
			}
			return nil
		})
	}
	sort.Strings(fonts)
	if len(fonts) > 20 {
		return fonts[:20]
	}
	return fonts
}

func mergePDFs(originalAbs, signPageAbs, signedAbs string) error {
	if err := os.MkdirAll(filepath.Dir(signedAbs), 0o755); err != nil {
		return fmt.Errorf("create signed pdf dir: %w", err)
	}
	if _, err := os.Stat(originalAbs); err != nil {
		return errors.New("file not found")
	}
	if _, err := os.Stat(signPageAbs); err != nil {
		return errors.New("file not found")
	}
	if path, err := exec.LookPath("pdfcpu"); err == nil {
		cmd := exec.Command(path, "merge", "-mode", "create", signedAbs, originalAbs, signPageAbs)
		if out, runErr := cmd.CombinedOutput(); runErr == nil {
			return nil
		} else {
			return fmt.Errorf("merge signed pdf: %v (%s)", runErr, strings.TrimSpace(string(out)))
		}
	}
	return ErrPDFCPUMissing
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
