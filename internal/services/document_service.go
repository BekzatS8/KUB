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
	"reflect"
	"regexp"
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
	GetByIDWithArchiveScope(id int64, scope repositories.ArchiveScope) (*models.Document, error)
	ListDocuments(limit, offset int) ([]*models.Document, error)
	ListDocumentsWithArchiveScope(limit, offset int, scope repositories.ArchiveScope) ([]*models.Document, error)
	ListDocumentsByDeal(dealID int64) ([]*models.Document, error)
	ListDocumentsByDealWithArchiveScope(dealID int64, scope repositories.ArchiveScope) ([]*models.Document, error)
	Delete(id int64) error
	Archive(id int64, archivedBy int, reason string) error
	Unarchive(id int64) error
	UpdateStatus(id int64, status string) error
	MarkSigned(id int64, signedBy string, signedAt time.Time) error
	Update(doc *models.Document) error
	UpdateSigningMeta(id int64, signMethod, signIP, signUserAgent, signMetadata string) error
}

type documentFilterRepo interface {
	ListDocumentsWithFilterAndArchiveScope(limit, offset int, filter repositories.DocumentListFilter, scope repositories.ArchiveScope) ([]*models.Document, error)
	ListDocumentsByDealWithFilterAndArchiveScope(dealID int64, filter repositories.DocumentListFilter, scope repositories.ArchiveScope) ([]*models.Document, error)
	CountDocumentsWithFilterAndArchiveScope(filter repositories.DocumentListFilter, scope repositories.ArchiveScope) (int, error)
	ListDocumentsByDealWithFilterAndArchiveScopePaginated(dealID int64, limit, offset int, filter repositories.DocumentListFilter, scope repositories.ArchiveScope) ([]*models.Document, error)
}

type LeadRepo interface {
	GetByID(id int) (*models.Leads, error)
}

type DealRepo interface {
	GetByID(id int) (*models.Deals, error)
	GetByLeadID(leadID int) (*models.Deals, error)
	GetLatestByClientID(clientID int) (*models.Deals, error)
	GetLatestByClientRef(clientID int, clientType string) (*models.Deals, error)
}

type ClientRepo interface {
	GetByID(id int) (*models.Client, error)
}

type DocumentService struct {
	DocRepo    DocumentRepo
	LeadRepo   LeadRepo
	DealRepo   DealRepo
	ClientRepo ClientRepo
	UserRepo   repositories.UserRepository
	SignSecret string

	FilesRoot string        // корень хранения файлов (cfg.Files.RootDir)
	PDFGen    pdf.Generator // генератор PDF (internal/pdf, txt-шаблоны/старые контракты)
	DocxGen   docx.Generator
	XlsxGen   xlsx.Generator
	now       func() time.Time
	displayTZ *time.Location
}

func (s *DocumentService) SetUserRepo(userRepo repositories.UserRepository) {
	s.UserRepo = userRepo
}

func (s *DocumentService) branchScopeForRole(userID, roleID int) (*int, error) {
	switch roleID {
	case authz.RoleSales, authz.RoleOperations, authz.RoleControl:
		if s.UserRepo == nil {
			return nil, nil
		}
		u, err := s.UserRepo.GetByID(userID)
		if err != nil || u == nil || u.BranchID == nil {
			return nil, errors.New("forbidden")
		}
		return u.BranchID, nil
	case authz.RoleManagement, authz.RoleSystemAdmin:
		return nil, nil
	default:
		return nil, errors.New("forbidden")
	}
}

func dealMatchesBranch(scope *int, deal *models.Deals) bool {
	if scope == nil {
		return true
	}
	if deal == nil || deal.BranchID == nil {
		return false
	}
	return *scope == *deal.BranchID
}

func (s *DocumentService) ensureDealAccess(deal *models.Deals, userID, roleID int) error {
	if deal == nil {
		return errors.New("not found")
	}
	if roleID == authz.RoleSales && deal.OwnerID != userID {
		return errors.New("forbidden")
	}
	branchScope, err := s.branchScopeForRole(userID, roleID)
	if err != nil {
		return err
	}
	if !dealMatchesBranch(branchScope, deal) {
		return errors.New("forbidden")
	}
	return nil
}

func (s *DocumentService) ResolveListBranchScope(userID, roleID int, requested *int64) (*int64, error) {
	branchScope, err := s.branchScopeForRole(userID, roleID)
	if err != nil {
		return nil, err
	}
	if branchScope != nil {
		v := int64(*branchScope)
		return &v, nil
	}
	return requested, nil
}

type SignerOverrides struct {
	Email    string
	FullName string
	Position string
	Phone    string
}

type ResolvedSigner struct {
	Email    string
	FullName string
	Position string
	Phone    string
}

type SigningContactOptions struct {
	DocumentID        int64    `json:"document_id"`
	ClientID          int      `json:"client_id"`
	DefaultPhone      string   `json:"default_phone"`
	DefaultEmail      string   `json:"default_email"`
	ResolvedFullName  string   `json:"resolved_full_name"`
	ResolvedPosition  string   `json:"resolved_position"`
	AvailableChannels []string `json:"available_channels"`
	PreferredChannel  string   `json:"preferred_channel,omitempty"`
}

type DocumentMissingFieldsError struct {
	Scope  string                 `json:"scope"`
	Fields []DocumentMissingField `json:"fields"`
}

type DocumentMissingField struct {
	Key    string     `json:"key"`
	Source FieldScope `json:"source"`
}

type DocumentUnresolvedPlaceholdersError struct {
	DocType      string   `json:"doc_type"`
	TemplateFile string   `json:"template_file"`
	MissingKeys  []string `json:"missing_keys"`
}

func (e *DocumentUnresolvedPlaceholdersError) Error() string { return "unresolved_placeholders" }

func (e *DocumentMissingFieldsError) Error() string { return "missing_fields" }

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
		now:        time.Now,
		displayTZ:  time.UTC,
	}
}

func (s *DocumentService) SetTimeProvider(now func() time.Time, displayTZ *time.Location) {
	if now != nil {
		s.now = now
	}
	if displayTZ != nil {
		s.displayTZ = displayTZ
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
	for key := range documentTypeRegistry {
		types[key] = struct{}{}
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

func (s *DocumentService) ListDocumentTypes() []DocumentTypeSpec {
	return ListDocumentTypeSpecs()
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

	if err := s.ensureDealAccess(deal, userID, roleID); err != nil {
		return 0, err
	}

	// статус по умолчанию
	if strings.TrimSpace(doc.Status) == "" {
		doc.Status = "draft"
	}
	if deal.BranchID != nil {
		v := int64(*deal.BranchID)
		doc.BranchID = &v
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
	if err := s.ensureDealAccess(deal, userID, roleID); err != nil {
		return nil, err
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
		BranchID: nil,
		DocType:  docType,
		FilePath: finalName,
		Status:   "draft",
	}
	if deal.BranchID != nil {
		v := int64(*deal.BranchID)
		doc.BranchID = &v
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
	if roleID != authz.RoleSales && roleID != authz.RoleOperations && roleID != authz.RoleControl {
		return doc, nil
	}
	if s.DealRepo == nil {
		return nil, errors.New("not found")
	}
	deal, derr := s.DealRepo.GetByID(int(doc.DealID))
	if derr != nil || deal == nil {
		return nil, errors.New("not found")
	}
	if err := s.ensureDealAccess(deal, userID, roleID); err != nil {
		if err.Error() == "forbidden" {
			return nil, errors.New("forbidden")
		}
		return nil, errors.New("not found")
	}
	return doc, nil
}

func (s *DocumentService) GetDocumentWithArchiveScope(id int64, userID, roleID int, scope repositories.ArchiveScope) (*models.Document, error) {
	doc, err := s.DocRepo.GetByIDWithArchiveScope(id, scope)
	if err != nil || doc == nil {
		return nil, err
	}
	if roleID != authz.RoleSales && roleID != authz.RoleOperations && roleID != authz.RoleControl {
		return doc, nil
	}
	if s.DealRepo == nil {
		return nil, errors.New("not found")
	}
	deal, derr := s.DealRepo.GetByID(int(doc.DealID))
	if derr != nil || deal == nil {
		return nil, errors.New("not found")
	}
	if err := s.ensureDealAccess(deal, userID, roleID); err != nil {
		if err.Error() == "forbidden" {
			return nil, errors.New("forbidden")
		}
		return nil, errors.New("not found")
	}
	return doc, nil
}

func (s *DocumentService) ResolveSignerEmail(id int64, userID, roleID int, fallbackEmail string) (string, error) {
	resolved, err := s.ResolveSignerForEmail(id, userID, roleID, SignerOverrides{Email: fallbackEmail})
	if err != nil {
		return "", err
	}
	return resolved.Email, nil
}

func (s *DocumentService) ResolveSigner(id int64, userID, roleID int, overrides SignerOverrides) (ResolvedSigner, error) {
	return s.ResolveSignerForEmail(id, userID, roleID, overrides)
}

func (s *DocumentService) ResolveSignerForEmail(id int64, userID, roleID int, overrides SignerOverrides) (ResolvedSigner, error) {
	result, err := s.resolveSignerBase(id, userID, roleID, overrides)
	if err != nil {
		return ResolvedSigner{}, err
	}
	if result.Email == "" {
		return ResolvedSigner{}, errors.New("signer email is required")
	}
	return result, nil
}

func (s *DocumentService) ResolveSignerForSMS(id int64, userID, roleID int, overrides SignerOverrides) (ResolvedSigner, error) {
	result, err := s.resolveSignerBase(id, userID, roleID, overrides)
	if err != nil {
		return ResolvedSigner{}, err
	}
	if result.Phone == "" {
		return ResolvedSigner{}, errors.New("signer phone is required")
	}
	return result, nil
}

func (s *DocumentService) ResolveSignerDefault(id int64, userID, roleID int, overrides SignerOverrides) (ResolvedSigner, error) {
	return s.resolveSignerBase(id, userID, roleID, overrides)
}

func (s *DocumentService) GetSigningContactOptions(id int64, userID, roleID int) (SigningContactOptions, error) {
	doc, err := s.GetDocument(id, userID, roleID)
	if err != nil || doc == nil {
		return SigningContactOptions{}, err
	}
	if s.DealRepo == nil {
		return SigningContactOptions{DocumentID: id}, nil
	}
	deal, err := s.DealRepo.GetByID(int(doc.DealID))
	if err != nil || deal == nil {
		return SigningContactOptions{}, err
	}
	resolved, err := s.ResolveSignerDefault(id, userID, roleID, SignerOverrides{})
	if err != nil {
		return SigningContactOptions{}, err
	}
	available := make([]string, 0, 2)
	preferred := ""
	if resolved.Phone != "" {
		available = append(available, "sms")
		preferred = "sms"
	}
	if resolved.Email != "" {
		available = append(available, "email")
		if preferred == "" {
			preferred = "email"
		}
	}
	return SigningContactOptions{
		DocumentID:        id,
		ClientID:          deal.ClientID,
		DefaultPhone:      resolved.Phone,
		DefaultEmail:      resolved.Email,
		ResolvedFullName:  resolved.FullName,
		ResolvedPosition:  resolved.Position,
		AvailableChannels: available,
		PreferredChannel:  preferred,
	}, nil
}

func (s *DocumentService) resolveSignerBase(id int64, userID, roleID int, overrides SignerOverrides) (ResolvedSigner, error) {
	doc, err := s.GetDocument(id, userID, roleID)
	if err != nil || doc == nil {
		return ResolvedSigner{}, err
	}
	overrides.Email = strings.TrimSpace(overrides.Email)
	overrides.FullName = strings.TrimSpace(overrides.FullName)
	overrides.Position = strings.TrimSpace(overrides.Position)
	overrides.Phone = strings.TrimSpace(overrides.Phone)
	normalizedOverridePhone := normalizePhone(overrides.Phone)
	if s.DealRepo == nil || s.ClientRepo == nil {
		return ResolvedSigner{Email: overrides.Email, FullName: overrides.FullName, Position: overrides.Position, Phone: normalizedOverridePhone}, nil
	}
	deal, err := s.DealRepo.GetByID(int(doc.DealID))
	if err != nil || deal == nil {
		return ResolvedSigner{Email: overrides.Email, FullName: overrides.FullName, Position: overrides.Position, Phone: normalizedOverridePhone}, nil
	}
	client, err := s.ClientRepo.GetByID(deal.ClientID)
	if err != nil || client == nil {
		return ResolvedSigner{Email: overrides.Email, FullName: overrides.FullName, Position: overrides.Position, Phone: normalizedOverridePhone}, nil
	}

	result := ResolvedSigner{
		Email:    overrides.Email,
		FullName: overrides.FullName,
		Position: overrides.Position,
		Phone:    normalizedOverridePhone,
	}
	if client.ClientType == models.ClientTypeLegal {
		if lp := client.LegalProfile; lp != nil {
			if result.Email == "" {
				result.Email = strings.TrimSpace(lp.ContactPersonEmail)
			}
			if result.FullName == "" {
				result.FullName = strings.TrimSpace(lp.DirectorFullName)
			}
			if result.FullName == "" {
				result.FullName = strings.TrimSpace(lp.ContactPersonName)
			}
			if result.Position == "" {
				result.Position = strings.TrimSpace(lp.ContactPersonPosition)
			}
			if result.Phone == "" {
				result.Phone = normalizePhone(strings.TrimSpace(lp.ContactPersonPhone))
			}
		}
		if result.Email == "" {
			result.Email = strings.TrimSpace(client.Email)
		}
		if result.Phone == "" {
			result.Phone = normalizePhone(strings.TrimSpace(client.Phone))
		}
	} else {
		if result.Email == "" {
			result.Email = strings.TrimSpace(client.Email)
		}
		if result.FullName == "" {
			result.FullName = strings.TrimSpace(client.Name)
		}
		if result.Phone == "" {
			result.Phone = normalizePhone(strings.TrimSpace(client.Phone))
		}
	}
	return result, nil
}

func (s *DocumentService) ListDocuments(limit, offset int) ([]*models.Document, error) {
	return s.DocRepo.ListDocumentsWithArchiveScope(limit, offset, repositories.ArchiveScopeActiveOnly)
}

func (s *DocumentService) ListDocumentsWithArchiveScope(limit, offset int, scope repositories.ArchiveScope) ([]*models.Document, error) {
	return s.DocRepo.ListDocumentsWithArchiveScope(limit, offset, scope)
}

func (s *DocumentService) ListDocumentsWithFilterAndArchiveScope(limit, offset int, filter repositories.DocumentListFilter, scope repositories.ArchiveScope) ([]*models.Document, error) {
	if repo, ok := s.DocRepo.(documentFilterRepo); ok {
		return repo.ListDocumentsWithFilterAndArchiveScope(limit, offset, filter, scope)
	}
	return s.DocRepo.ListDocumentsWithArchiveScope(limit, offset, scope)
}

func (s *DocumentService) ListDocumentsByDeal(dealID int64, userID, roleID int, scope repositories.ArchiveScope) ([]*models.Document, error) {
	return s.ListDocumentsByDealWithFilter(dealID, userID, roleID, repositories.DocumentListFilter{}, scope)
}

func (s *DocumentService) ListDocumentsByDealWithFilter(dealID int64, userID, roleID int, filter repositories.DocumentListFilter, scope repositories.ArchiveScope) ([]*models.Document, error) {
	deal, err := s.DealRepo.GetByID(int(dealID))
	if err != nil || deal == nil {
		return nil, errors.New("not found")
	}
	if err := s.ensureDealAccess(deal, userID, roleID); err != nil {
		return nil, err
	}
	if repo, ok := s.DocRepo.(documentFilterRepo); ok {
		return repo.ListDocumentsByDealWithFilterAndArchiveScope(dealID, filter, scope)
	}
	return s.DocRepo.ListDocumentsByDealWithArchiveScope(dealID, scope)
}

func (s *DocumentService) ListDocumentsWithFilterAndArchiveScopeAndTotal(limit, offset int, filter repositories.DocumentListFilter, scope repositories.ArchiveScope) ([]*models.Document, int, error) {
	items, err := s.ListDocumentsWithFilterAndArchiveScope(limit, offset, filter, scope)
	if err != nil {
		return nil, 0, err
	}
	repo, ok := s.DocRepo.(documentFilterRepo)
	if !ok {
		return items, len(items), nil
	}
	total, err := repo.CountDocumentsWithFilterAndArchiveScope(filter, scope)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *DocumentService) ListDocumentsByDealWithFilterAndTotal(dealID int64, userID, roleID, limit, offset int, filter repositories.DocumentListFilter, scope repositories.ArchiveScope) ([]*models.Document, int, error) {
	deal, err := s.DealRepo.GetByID(int(dealID))
	if err != nil || deal == nil {
		return nil, 0, errors.New("not found")
	}
	if err := s.ensureDealAccess(deal, userID, roleID); err != nil {
		return nil, 0, err
	}
	repo, ok := s.DocRepo.(documentFilterRepo)
	if !ok {
		items, listErr := s.ListDocumentsByDealWithFilter(dealID, userID, roleID, filter, scope)
		if listErr != nil {
			return nil, 0, listErr
		}
		if items == nil {
			items = make([]*models.Document, 0)
		}
		return items, len(items), nil
	}
	items, err := repo.ListDocumentsByDealWithFilterAndArchiveScopePaginated(dealID, limit, offset, filter, scope)
	if err != nil {
		return nil, 0, err
	}
	if items == nil {
		items = make([]*models.Document, 0)
	}
	filter.DealID = &dealID
	total, err := repo.CountDocumentsWithFilterAndArchiveScope(filter, scope)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *DocumentService) DeleteDocument(id int64, userID, roleID int) error {
	if !authz.CanHardDeleteBusinessEntity(roleID) {
		return errors.New("forbidden")
	}
	doc, err := s.DocRepo.GetByIDWithArchiveScope(id, repositories.ArchiveScopeAll)
	if err != nil || doc == nil {
		return errors.New("not found")
	}
	deal, derr := s.DealRepo.GetByID(int(doc.DealID))
	if derr != nil || deal == nil {
		return errors.New("not found")
	}
	if err := s.ensureDealAccess(deal, userID, roleID); err != nil {
		return err
	}
	if err := s.deleteDocumentFiles(doc); err != nil {
		return err
	}
	return s.DocRepo.Delete(id)
}

func (s *DocumentService) ArchiveDocument(id int64, userID, roleID int, reason string) error {
	if !authz.CanArchiveBusinessEntity(roleID) {
		return errors.New("forbidden")
	}
	doc, err := s.DocRepo.GetByIDWithArchiveScope(id, repositories.ArchiveScopeAll)
	if err != nil || doc == nil {
		return errors.New("not found")
	}
	deal, derr := s.DealRepo.GetByID(int(doc.DealID))
	if derr != nil || deal == nil {
		return errors.New("not found")
	}
	if err := s.ensureDealAccess(deal, userID, roleID); err != nil {
		return err
	}
	if doc.IsArchived {
		return nil
	}
	return s.DocRepo.Archive(id, userID, reason)
}

func (s *DocumentService) UnarchiveDocument(id int64, userID, roleID int) error {
	if !authz.CanArchiveBusinessEntity(roleID) {
		return errors.New("forbidden")
	}
	doc, err := s.DocRepo.GetByIDWithArchiveScope(id, repositories.ArchiveScopeAll)
	if err != nil || doc == nil {
		return errors.New("not found")
	}
	deal, derr := s.DealRepo.GetByID(int(doc.DealID))
	if derr != nil || deal == nil {
		return errors.New("not found")
	}
	if err := s.ensureDealAccess(deal, userID, roleID); err != nil {
		return err
	}
	if !doc.IsArchived {
		return errors.New("not archived")
	}
	return s.DocRepo.Unarchive(id)
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
	if err := s.ensureDealAccess(deal, userID, roleID); err != nil {
		return err
	}
	if doc.Status != "draft" {
		return errors.New("invalid status")
	}
	return s.DocRepo.UpdateStatus(id, "under_review")
}

func (s *DocumentService) Review(id int64, action string, userID, roleID int) error {
	if !authz.CanProcessDocuments(roleID) {
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
	return s.DocRepo.MarkSigned(id, "", time.Now())
}

func (s *DocumentService) MarkDocumentSigned(id int64, signedBy string, signedAt *time.Time, userID, roleID int) error {
	if roleID != authz.RoleManagement {
		return errors.New("forbidden")
	}
	doc, err := s.DocRepo.GetByID(id)
	if err != nil || doc == nil {
		return errors.New("not found")
	}
	if !(doc.Status == "approved" || doc.Status == "returned" || doc.Status == "sent_for_signature") {
		return errors.New("invalid status")
	}
	ts := time.Now()
	if signedAt != nil {
		ts = *signedAt
	}
	return s.DocRepo.MarkSigned(id, strings.TrimSpace(signedBy), ts)
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
		return nil, errors.New("unsupported_doc_type_for_lead_use_create_from_client")
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
	clientType string,
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

	spec, hasSpec := GetDocumentTypeSpec(docType)
	if !hasSpec {
		return nil, errors.New("unsupported doc_type")
	}

	// --- Клиент ---
	normalizedClientType, err := normalizeRequiredDealClientType(clientType)
	if err != nil {
		return nil, err
	}
	client, err := s.ClientRepo.GetByID(clientID)
	if err != nil || client == nil {
		return nil, errors.New("client not found")
	}
	storedClientType, err := normalizeRequiredDealClientType(client.ClientType)
	if err != nil {
		return nil, err
	}
	if storedClientType != normalizedClientType {
		return nil, ErrClientTypeMismatch
	}

	// --- Нужна ли сделка? ---
	needDeal := true

	var deal *models.Deals
	if needDeal {
		if dealID > 0 {
			deal, err = s.DealRepo.GetByID(dealID)
			if err != nil || deal == nil {
				return nil, errors.New("deal not found")
			}
			if deal.ClientID != clientID {
				return nil, errors.New("deal does not belong to client")
			}
			if strings.TrimSpace(deal.ClientType) != "" && strings.ToLower(strings.TrimSpace(deal.ClientType)) != normalizedClientType {
				return nil, ErrClientTypeMismatch
			}
		} else {
			deal, err = s.DealRepo.GetLatestByClientRef(clientID, normalizedClientType)
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
	dealExtra := buildDealExtraFallback(deal)
	mergedExtra := mergeExtra(dealExtra, extra)
	selectedTemplate := selectTemplateForClient(spec, client)

	// базовые плейсхолдеры
	placeholders := buildClientPlaceholders(client, deal, mergedExtra, now)

	applyReasonCheckboxes(docType, mergedExtra, placeholders)

	missingClient := mapClientMissing(docType, validateClientFieldsForDocType(docType, client))
	required := requiredFieldsForClientType(spec, client, selectedTemplate)
	missing := append(missingClient, validateRequiredByPlaceholders(required, placeholders)...)
	if len(missing) > 0 {
		return nil, &DocumentMissingFieldsError{Scope: docType, Fields: missing}
	}

	// базовое имя файла без расширения
	baseFilename := fmt.Sprintf(
		"%s_client_%d_%s",
		docType,
		client.ID,
		now.Format("20060102_150405"),
	)

	// ================== EXCEL ==================
	if spec.Format == DocumentFormatXLSX {
		if s.XlsxGen == nil {
			return nil, errors.New("xlsx generator not configured")
		}

		excelRelPath, excelPDFPath, err := s.XlsxGen.GenerateFromTemplateAndPDF(selectedTemplate, placeholders, baseFilename)
		if err != nil {
			return nil, wrapGenerationError(docType, err)
		}

		excelRelPath = normalizeStoragePath(excelRelPath)

		doc := &models.Document{
			DealID:       getDealID64(deal),
			DocType:      docType,
			Status:       "draft",
			FilePath:     excelRelPath,
			FilePathPdf:  normalizeStoragePath(excelPDFPath),
			FilePathDocx: "",
		}

		id, err := s.DocRepo.Create(doc)
		if err != nil {
			return nil, err
		}
		doc.ID = id
		return doc, nil
	}

	if spec.Format == DocumentFormatDOCX && s.DocxGen == nil {
		return nil, errors.New("pdf_conversion_disabled")
	}
	// ================== DOCX ==================
	if spec.Format == DocumentFormatDOCX {
		docxRelPath, pdfRelPath, err := s.DocxGen.GenerateDocxAndPDF(
			selectedTemplate,
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

	return nil, errors.New("unsupported doc_type")
}

func validateClientFieldsForDocType(docType string, c *models.Client) (missing []string) {
	spec, ok := GetDocumentTypeSpec(docType)
	if !ok {
		return nil
	}
	if c == nil {
		base := []string{"full_name", "iin_or_bin", "address", "phone"}
		for _, req := range spec.RequiredFields {
			if req.Scope == FieldScopeClient && req.Required {
				found := false
				for _, e := range base {
					if e == req.Key {
						found = true
						break
					}
				}
				if !found {
					base = append(base, req.Key)
				}
			}
		}
		return base
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
	if strings.TrimSpace(c.Phone) == "" {
		add("phone")
	}
	if c.ClientType == models.ClientTypeLegal {
		company := strings.TrimSpace(c.Name)
		bin := strings.TrimSpace(c.BinIin)
		legalAddress := strings.TrimSpace(c.Address)
		signerName := ""
		signerPosition := ""
		signerEmail := strings.TrimSpace(c.Email)
		signerPhone := strings.TrimSpace(c.Phone)
		if c.LegalProfile != nil {
			if v := strings.TrimSpace(c.LegalProfile.CompanyName); v != "" {
				company = v
			}
			if v := strings.TrimSpace(c.LegalProfile.BIN); v != "" {
				bin = v
			}
			if v := strings.TrimSpace(c.LegalProfile.LegalAddress); v != "" {
				legalAddress = v
			}
			signerName = strings.TrimSpace(c.LegalProfile.DirectorFullName)
			if signerName == "" {
				signerName = strings.TrimSpace(c.LegalProfile.ContactPersonName)
			}
			signerPosition = strings.TrimSpace(c.LegalProfile.ContactPersonPosition)
			if v := strings.TrimSpace(c.LegalProfile.ContactPersonEmail); v != "" {
				signerEmail = v
			}
			if v := strings.TrimSpace(c.LegalProfile.ContactPersonPhone); v != "" {
				signerPhone = v
			}
		}
		if company == "" {
			add("company_name")
		}
		if bin == "" {
			add("bin")
		}
		if legalAddress == "" {
			add("legal_address")
		}
		if signerName == "" {
			add("signer_full_name")
		}
		if signerPosition == "" {
			add("signer_position")
		}
		if signerEmail == "" && signerPhone == "" {
			add("signer_email")
		}
	}
	for _, req := range spec.RequiredFields {
		if req.Scope != FieldScopeClient || !req.Required {
			continue
		}
		if req.Key == "id_number" && strings.TrimSpace(c.IDNumber) == "" {
			add("id_number")
		}
		if req.Key == "passport_number" && strings.TrimSpace(c.PassportNumber) == "" {
			add("passport_number")
		}
	}
	return missing
}

func selectTemplateForClient(spec DocumentTypeSpec, client *models.Client) string {
	if client != nil && client.ClientType == models.ClientTypeLegal && strings.TrimSpace(spec.LegalTemplate) != "" {
		return spec.LegalTemplate
	}
	return spec.TemplateFile
}

func requiredFieldsForClientType(spec DocumentTypeSpec, client *models.Client, templateFile string) DocumentTypeSpec {
	result := spec
	result.TemplateFile = templateFile
	if client == nil || client.ClientType != models.ClientTypeLegal {
		return result
	}
	legalRequired := []DocumentFieldRequirement{
		{Key: "LEGAL_COMPANY_NAME", Scope: FieldScopeClient, Required: true},
		{Key: "LEGAL_BIN", Scope: FieldScopeClient, Required: true},
		{Key: "LEGAL_LEGAL_ADDRESS", Scope: FieldScopeClient, Required: true},
		{Key: "SIGNER_FULL_NAME", Scope: FieldScopeClient, Required: true},
		{Key: "SIGNER_POSITION", Scope: FieldScopeClient, Required: true},
		{Key: "signer_contact", Scope: FieldScopeClient, Required: true},
	}
	if requiresLegalBankDetails(spec.DocType) {
		legalRequired = append(legalRequired,
			DocumentFieldRequirement{Key: "LEGAL_BANK_NAME", Scope: FieldScopeClient, Required: true},
			DocumentFieldRequirement{Key: "LEGAL_IBAN", Scope: FieldScopeClient, Required: true},
			DocumentFieldRequirement{Key: "LEGAL_BIK", Scope: FieldScopeClient, Required: true},
			DocumentFieldRequirement{Key: "LEGAL_KBE", Scope: FieldScopeClient, Required: true},
		)
	}
	result.RequiredFields = append(result.RequiredFields, legalRequired...)
	return result
}

func requiresLegalBankDetails(docType string) bool {
	switch normalizeDocType(docType) {
	case "contract_paid_full_ru", "contract_paid_50_50_ru", "contract_language_courses":
		return true
	default:
		return false
	}
}

func validateRequiredByPlaceholders(spec DocumentTypeSpec, placeholders map[string]string) []DocumentMissingField {
	missing := make([]DocumentMissingField, 0)
	for _, req := range spec.RequiredFields {
		if !req.Required {
			continue
		}
		if strings.TrimSpace(resolveRequiredValue(req, placeholders)) == "" {
			missing = append(missing, DocumentMissingField{Key: req.Key, Source: req.Scope})
		}
	}
	return missing
}

func resolveRequiredValue(req DocumentFieldRequirement, placeholders map[string]string) string {
	key := strings.TrimSpace(req.Key)
	if key == "" {
		return ""
	}
	if v := strings.TrimSpace(placeholders[key]); v != "" {
		return v
	}
	switch key {
	case "full_name":
		return placeholders["CLIENT_FULL_NAME"]
	case "iin_or_bin":
		if v := strings.TrimSpace(placeholders["CLIENT_IIN"]); v != "" {
			return v
		}
		return placeholders["CLIENT_BIN_IIN"]
	case "address":
		if v := strings.TrimSpace(placeholders["CLIENT_ADDRESS"]); v != "" {
			return v
		}
		if v := strings.TrimSpace(placeholders["CLIENT_FACT_ADDRESS"]); v != "" {
			return v
		}
		return placeholders["CLIENT_REG_ADDRESS"]
	case "phone":
		return placeholders["CLIENT_PHONE"]
	case "contract_number":
		return placeholders["CONTRACT_NUMBER"]
	case "id_number":
		return placeholders["CLIENT_ID_NUMBER"]
	case "passport_number":
		return placeholders["CLIENT_PASSPORT_NUMBER"]
	case "reason_code":
		return placeholders["reason_code"]
	case "signer_contact":
		if v := strings.TrimSpace(placeholders["SIGNER_EMAIL"]); v != "" {
			return v
		}
		return placeholders["SIGNER_PHONE"]
	default:
		return placeholders[key]
	}
}

func mergeExtra(dealExtra, reqExtra map[string]string) map[string]string {
	merged := make(map[string]string, len(dealExtra)+len(reqExtra))
	for k, v := range dealExtra {
		merged[k] = v
	}
	for k, v := range reqExtra {
		if strings.TrimSpace(v) == "" {
			continue
		}
		merged[k] = v
	}
	return merged
}

func buildDealExtraFallback(deal *models.Deals) map[string]string {
	if deal == nil {
		return map[string]string{}
	}
	m := map[string]string{
		"CONTRACT_NUMBER": fmt.Sprintf("KUB-%06d", deal.ID),
		"contract_number": fmt.Sprintf("KUB-%06d", deal.ID),
	}
	if deal.CreatedAt.Unix() > 0 {
		m["CONTRACT_DATE"] = deal.CreatedAt.Format("02.01.2006")
		m["CONTRACT_DATE_RAW"] = deal.CreatedAt.Format("02.01.2006")
		m["DOC_DATE"] = deal.CreatedAt.Format("02.01.2006")
	}
	if deal.Amount > 0 {
		amount := strconv.FormatFloat(deal.Amount, 'f', 2, 64)
		m["TOTAL_AMOUNT_NUM"] = amount
		m["DEAL_AMOUNT_NUM"] = amount
		m["REFUND_AMOUNT_NUM"] = amount
		m["PREPAY_AMOUNT_NUM"] = amount
	}
	if strings.TrimSpace(deal.Currency) != "" {
		m["DEAL_CURRENCY"] = strings.TrimSpace(deal.Currency)
	}
	for k, v := range parseDealExtraJSON(deal) {
		if strings.TrimSpace(v) == "" {
			continue
		}
		m[k] = v
	}
	return m
}

func parseDealExtraJSON(deal *models.Deals) map[string]string {
	result := map[string]string{}
	if deal == nil {
		return result
	}
	rv := reflect.ValueOf(deal)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if !rv.IsValid() || rv.Kind() != reflect.Struct {
		return result
	}

	raw := ""
	for _, fieldName := range []string{"ExtraJSON", "ExtraJson", "DealExtraJSON", "DealExtraJson"} {
		f := rv.FieldByName(fieldName)
		if f.IsValid() && f.Kind() == reflect.String {
			raw = strings.TrimSpace(f.String())
			if raw != "" {
				break
			}
		}
	}
	if raw == "" {
		return result
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return result
	}
	for k, v := range payload {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		val := strings.TrimSpace(fmt.Sprint(v))
		if val == "" || val == "<nil>" {
			continue
		}
		result[key] = val
	}
	return result
}

func mapClientMissing(docType string, fields []string) []DocumentMissingField {
	spec, ok := GetDocumentTypeSpec(docType)
	res := make([]DocumentMissingField, 0, len(fields))
	for _, f := range fields {
		src := FieldScopeClient
		if ok {
			for _, req := range spec.RequiredFields {
				if req.Key == f {
					src = req.Scope
					break
				}
			}
		}
		res = append(res, DocumentMissingField{Key: f, Source: src})
	}
	return res
}

func applyReasonCheckboxes(docType string, extra map[string]string, placeholders map[string]string) {
	reasonCodes := collectReasonCodes(extra)

	applyReasonSet := func(prefix string, max int, checked string, unchecked string) {
		flags := make([]string, 0, max)
		for i := 1; i <= max; i++ {
			flags = append(flags, fmt.Sprintf("%s_R%d", prefix, i))
		}

		hasExplicit := false
		for _, k := range flags {
			if _, ok := extra[k]; ok {
				hasExplicit = true
				break
			}
		}

		if hasExplicit {
			for _, k := range flags {
				if v, ok := extra[k]; ok {
					placeholders[k] = strings.TrimSpace(v)
				} else {
					placeholders[k] = unchecked
				}
			}
			return
		}

		for _, k := range flags {
			placeholders[k] = unchecked
		}
		for code := range reasonCodes {
			key := fmt.Sprintf("%s_%s", prefix, code)
			for _, candidate := range flags {
				if candidate == key {
					placeholders[candidate] = checked
				}
			}
		}
	}

	switch docType {
	case "refund_application":
		applyReasonSet("REFUND", 6, "☑", "☐")
	case "pause_application":
		applyReasonSet("PAUSE", 8, "X", "")
	case "cancel_appointment":
		applyReasonSet("CANCEL", 8, "☑", "☐")
	case "documents_handover_act":
		applyDocsMarks(extra, placeholders, 15)
	}
}

func applyDocsMarks(extra map[string]string, placeholders map[string]string, max int) {
	selected := parseIntSelectionList(extra["DOCS_PRESENT"])
	for i := 1; i <= max; i++ {
		key := fmt.Sprintf("DOCS_MARK_%d", i)
		if explicit, ok := extra[key]; ok {
			placeholders[key] = strings.TrimSpace(explicit)
			continue
		}
		if _, ok := selected[i]; ok {
			placeholders[key] = "✓"
		} else {
			placeholders[key] = ""
		}
	}
}

func collectReasonCodes(extra map[string]string) map[string]struct{} {
	codes := map[string]struct{}{}
	multi := parseReasonCodeList(extra["reason_codes"])
	if len(multi) > 0 {
		for _, code := range multi {
			codes[code] = struct{}{}
		}
		return codes
	}
	for _, code := range parseReasonCodeList(extra["reason_code"]) {
		codes[code] = struct{}{}
	}
	for _, code := range parseReasonCodeList(extra["REFUND_REASON_CODE"]) {
		codes[code] = struct{}{}
	}
	return codes
}

func parseReasonCodeList(raw string) []string {
	items := parseStringList(raw)
	res := make([]string, 0, len(items))
	for _, item := range items {
		code := strings.ToUpper(strings.TrimSpace(item))
		if matched, _ := regexp.MatchString(`^R[0-9]+$`, code); matched {
			res = append(res, code)
		}
	}
	return res
}

func parseIntSelectionList(raw string) map[int]struct{} {
	res := map[int]struct{}{}
	for _, item := range parseStringList(raw) {
		n, err := strconv.Atoi(strings.TrimSpace(item))
		if err == nil {
			res[n] = struct{}{}
		}
	}
	return res
}

func parseStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var arrAny []any
		if err := json.Unmarshal([]byte(raw), &arrAny); err == nil {
			out := make([]string, 0, len(arrAny))
			for _, item := range arrAny {
				out = append(out, strings.TrimSpace(fmt.Sprint(item)))
			}
			return out
		}
	}
	if strings.Contains(raw, ",") {
		parts := strings.Split(raw, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			out = append(out, strings.TrimSpace(p))
		}
		return out
	}
	return []string{raw}
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
	var docxUnresolved *docx.UnresolvedPlaceholdersError
	if errors.As(err, &docxUnresolved) {
		return &DocumentUnresolvedPlaceholdersError{
			DocType:      docType,
			TemplateFile: docxUnresolved.TemplateFile,
			MissingKeys:  docxUnresolved.MissingKeys,
		}
	}
	var xlsxUnresolved *xlsx.UnresolvedPlaceholdersError
	if errors.As(err, &xlsxUnresolved) {
		return &DocumentUnresolvedPlaceholdersError{
			DocType:      docType,
			TemplateFile: xlsxUnresolved.TemplateFile,
			MissingKeys:  xlsxUnresolved.MissingKeys,
		}
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(msg, "template not found") {
		return errors.New("template_not_found")
	}
	if strings.Contains(msg, "pdf_conversion_disabled") {
		return errors.New("pdf_conversion_disabled")
	}
	if strings.Contains(msg, "conversion is disabled") {
		return errors.New("pdf_conversion_disabled")
	}
	if strings.Contains(msg, "pdf_conversion_failed") || strings.Contains(msg, "libreoffice conversion") {
		return errors.New("pdf_conversion_failed")
	}
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
		signedAt = formatSignedAtForSigningSheet(session.SignedAt, s.displayTZ)
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
	hashValue := strings.TrimSpace(session.DocHash)
	if _, _, hashOK := splitSHA256(hashValue); !hashOK {
		hashValue = "—"
	}
	fontSize := 10.0
	lineH := fontSize * 1.3
	spacing := 1.0
	label := "Хэш документа (SHA-256):"
	gap := 5.0
	x, y := pdfFile.GetXY()
	leftMargin, _, rightMargin, _ := pdfFile.GetMargins()
	pageW, _ := pdfFile.GetPageSize()
	pdfFile.SetFont("dejavu", "B", fontSize)
	wLabel := pdfFile.GetStringWidth(label)
	pdfFile.SetXY(x, y)
	pdfFile.CellFormat(wLabel, lineH, label, "", 0, "L", false, 0, "")

	// Раздельные блоки и MultiCell предотвращают наложение хеша на лейбл и корректно переносят длинное значение.
	xHash := x + wLabel + gap
	if xHash < leftMargin {
		xHash = leftMargin
	}
	wHash := pageW - rightMargin - xHash
	if wHash < 10 {
		wHash = 10
	}
	pdfFile.SetFont("dejavu", "", fontSize)
	pdfFile.SetXY(xHash, y)
	pdfFile.MultiCell(wHash, lineH, hashValue, "", "L", false)
	y = pdfFile.GetY() + spacing
	pdfFile.SetXY(x, y)

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

func formatSignedAtForSigningSheet(signedAt *time.Time, loc *time.Location) string {
	if signedAt == nil {
		return ""
	}
	if loc == nil {
		loc = time.UTC
	}
	tzLabel := loc.String()
	if tzLabel == "" || tzLabel == "UTC" {
		tzLabel = signedAt.In(loc).Format("-07:00")
	}
	return fmt.Sprintf("%s (%s)", signedAt.In(loc).Format("02.01.2006 15:04"), tzLabel)
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
