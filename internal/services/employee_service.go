package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"turcompany/internal/authz"
	"turcompany/internal/docx"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/storage"
)

// EmployeeService — раздел «Сотрудники» и кадровые документы с онлайн-
// подписанием (ТЗ «доработки CRM КУБ», п.1–2).
//
// Доступ к разделу (и к зарплатам) — администратор, руководство, отдел
// кадров. Шаблоны загружают администратор и отдел кадров. Кадровый документ —
// строка общей таблицы documents (scope = 'hr'), поэтому подписание кодом из
// SMS/email идёт тем же механизмом, что и у клиентов.
type EmployeeService struct {
	repo        *repositories.EmployeeRepository
	store       storage.Storage
	filesRoot   string
	signer      EmployeeDocSigner
	officeOn    bool
	officeBin   string
	now         func() time.Time
	displayZone *time.Location
}

// EmployeeDocSigner — отправка документа на подпись (DocumentSigningConfirmationService).
type EmployeeDocSigner interface {
	StartSigningBySMS(ctx context.Context, documentID, userID int64, signerPhone, signerEmail string) (*SigningStartResult, error)
	StartSigning(ctx context.Context, documentID, userID int64, signerEmail string) (*SigningStartResult, error)
}

func NewEmployeeService(repo *repositories.EmployeeRepository, store storage.Storage, filesRoot string, signer EmployeeDocSigner, officeEnabled bool, officeBinary string) *EmployeeService {
	loc, err := time.LoadLocation("Asia/Almaty")
	if err != nil {
		loc = time.FixedZone("UTC+5", 5*3600)
	}
	return &EmployeeService{
		repo: repo, store: store, filesRoot: filesRoot, signer: signer,
		officeOn: officeEnabled, officeBin: officeBinary, now: time.Now, displayZone: loc,
	}
}

// EmployeeValidationError — неверные данные; Msg — для пользователя.
type EmployeeValidationError struct{ Msg string }

func (e *EmployeeValidationError) Error() string { return e.Msg }

func invalid(msg string) error { return &EmployeeValidationError{Msg: msg} }

// CanManageEmployees — раздел «Сотрудники» и зарплаты.
func CanManageEmployees(roleID int) bool {
	return roleID == authz.RoleSystemAdmin || roleID == authz.RoleManagement || roleID == authz.RoleHR
}

// CanManageHRTemplates — загрузка шаблонов кадровых документов.
func CanManageHRTemplates(roleID int) bool {
	return roleID == authz.RoleSystemAdmin || roleID == authz.RoleHR
}

var iinPattern = regexp.MustCompile(`^[0-9]{12}$`)

// normalizeKZPhone приводит телефон к виду «+7 XXX XXX XX XX».
func normalizeKZPhone(raw string) (string, bool) {
	d := normalizePhone(raw)
	if len(d) == 10 {
		d = "7" + d
	}
	if len(d) == 11 && d[0] == '8' {
		d = "7" + d[1:]
	}
	if len(d) != 11 || d[0] != '7' {
		return "", false
	}
	return fmt.Sprintf("+7 %s %s %s %s", d[1:4], d[4:7], d[7:9], d[9:11]), true
}

func parseDate(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	return t, err == nil
}

// normalizeEmployee проверяет обязательные поля и правило «дата увольнения →
// тип „Уволенный“».
func normalizeEmployee(e *models.Employee) error {
	e.FullName = strings.Join(strings.Fields(e.FullName), " ")
	e.IIN = strings.TrimSpace(e.IIN)
	e.Address = strings.TrimSpace(e.Address)
	e.Email = strings.TrimSpace(e.Email)
	e.EmergencyName = strings.TrimSpace(e.EmergencyName)
	e.EmergencyRelation = strings.TrimSpace(e.EmergencyRelation)
	e.Position = strings.TrimSpace(e.Position)
	e.Type = strings.TrimSpace(e.Type)
	e.FiredAt = strings.TrimSpace(e.FiredAt)

	switch {
	case e.FullName == "":
		return invalid("Укажите ФИО")
	case e.Address == "":
		return invalid("Укажите адрес проживания")
	case e.EmergencyName == "" || e.EmergencyRelation == "":
		return invalid("Укажите экстренный контакт: ФИО и кем приходится")
	case e.Position == "":
		return invalid("Укажите должность")
	}
	if !iinPattern.MatchString(e.IIN) {
		return invalid("ИИН — 12 цифр")
	}
	birth, ok := parseDate(e.BirthDate)
	if !ok {
		return invalid("Укажите дату рождения")
	}
	if birth.After(time.Now()) {
		return invalid("Дата рождения в будущем")
	}
	hired, ok := parseDate(e.HiredAt)
	if !ok {
		return invalid("Укажите дату начала работы")
	}
	phone, ok := normalizeKZPhone(e.Phone)
	if !ok {
		return invalid("Телефон в формате +7 XXX XXX XX XX")
	}
	e.Phone = phone
	ePhone, ok := normalizeKZPhone(e.EmergencyPhone)
	if !ok {
		return invalid("Телефон экстренного контакта в формате +7 XXX XXX XX XX")
	}
	e.EmergencyPhone = ePhone
	if e.Email != "" && !strings.Contains(e.Email, "@") {
		return invalid("Неверный email")
	}
	if e.Salary <= 0 || math.IsNaN(e.Salary) || math.IsInf(e.Salary, 0) {
		return invalid("Укажите заработную плату")
	}
	e.Salary = math.Round(e.Salary*100) / 100

	switch e.Type {
	case models.EmployeeTypePermanent, models.EmployeeTypeIntern, models.EmployeeTypeFired:
	case "":
		e.Type = models.EmployeeTypePermanent
	default:
		return invalid("Тип: постоянный сотрудник, стажёр или уволенный")
	}
	if e.FiredAt != "" {
		fired, ok := parseDate(e.FiredAt)
		if !ok {
			return invalid("Неверная дата увольнения")
		}
		if fired.Before(hired) {
			return invalid("Дата увольнения раньше даты начала работы")
		}
		// При заполнении даты увольнения тип автоматически — «Уволенный».
		e.Type = models.EmployeeTypeFired
	} else if e.Type == models.EmployeeTypeFired {
		// «Уволенный» без даты — увольнение сегодняшним днём.
		e.FiredAt = time.Now().Format("2006-01-02")
	}
	if e.UserID != nil && *e.UserID <= 0 {
		e.UserID = nil
	}
	if e.BranchID != nil && *e.BranchID <= 0 {
		e.BranchID = nil
	}
	return nil
}

func (s *EmployeeService) List(ctx context.Context, f models.EmployeeFilter, roleID int) ([]*models.Employee, int, error) {
	if !CanManageEmployees(roleID) {
		return nil, 0, ErrForbidden
	}
	return s.repo.List(ctx, f)
}

func (s *EmployeeService) Positions(ctx context.Context, roleID int) ([]string, error) {
	if !CanManageEmployees(roleID) {
		return nil, ErrForbidden
	}
	return s.repo.Positions(ctx)
}

func (s *EmployeeService) Get(ctx context.Context, id, roleID int) (*models.Employee, error) {
	if !CanManageEmployees(roleID) {
		return nil, ErrForbidden
	}
	e, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrNotFound
	}
	return e, nil
}

func (s *EmployeeService) Create(ctx context.Context, e *models.Employee, userID, roleID int) (*models.Employee, error) {
	if !CanManageEmployees(roleID) {
		return nil, ErrForbidden
	}
	if err := normalizeEmployee(e); err != nil {
		return nil, err
	}
	id, err := s.repo.Create(ctx, e, userID)
	if err != nil {
		if errors.Is(err, repositories.ErrEmployeeIINTaken) {
			return nil, invalid("Сотрудник с таким ИИН уже есть")
		}
		return nil, err
	}
	return s.repo.GetByID(ctx, id)
}

// Update — правка карточки. Уволенный сотрудник остаётся в базе со всеми
// документами: удаления нет, увольнение — это дата и тип.
func (s *EmployeeService) Update(ctx context.Context, e *models.Employee, roleID int) (*models.Employee, error) {
	if !CanManageEmployees(roleID) {
		return nil, ErrForbidden
	}
	current, err := s.repo.GetByID(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrNotFound
	}
	// Сняли дату увольнения у уволенного — вернули на работу постоянным.
	if strings.TrimSpace(e.FiredAt) == "" && e.Type == models.EmployeeTypeFired && current.FiredAt != "" {
		e.Type = models.EmployeeTypePermanent
	}
	if err := normalizeEmployee(e); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, e); err != nil {
		if errors.Is(err, repositories.ErrEmployeeIINTaken) {
			return nil, invalid("Сотрудник с таким ИИН уже есть")
		}
		return nil, err
	}
	return s.repo.GetByID(ctx, e.ID)
}

const employeePhotoMaxBytes = 5 << 20

// SetPhoto — фото JPG/PNG до 5 МБ.
func (s *EmployeeService) SetPhoto(ctx context.Context, id int, content []byte, roleID int) error {
	if !CanManageEmployees(roleID) {
		return ErrForbidden
	}
	if len(content) == 0 {
		return invalid("Пустой файл")
	}
	if len(content) > employeePhotoMaxBytes {
		return invalid("Фото — не больше 5 МБ")
	}
	ext := ""
	switch {
	case bytes.HasPrefix(content, []byte{0xFF, 0xD8, 0xFF}):
		ext = ".jpg"
	case bytes.HasPrefix(content, []byte{0x89, 'P', 'N', 'G'}):
		ext = ".png"
	default:
		return invalid("Фото — JPG или PNG")
	}
	e, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrNotFound
	}
	key := fmt.Sprintf("employees/%d/photo_%d%s", id, s.now().UnixNano(), ext)
	if err := s.store.Save(ctx, bytes.NewReader(content), key); err != nil {
		return fmt.Errorf("save employee photo: %w", err)
	}
	if err := s.repo.SetPhoto(ctx, id, key); err != nil {
		_ = s.store.Delete(ctx, key)
		return err
	}
	if e.PhotoKey != "" {
		_ = s.store.Delete(ctx, e.PhotoKey)
	}
	return nil
}

// OpenPhoto — содержимое фото и его тип.
func (s *EmployeeService) OpenPhoto(ctx context.Context, id, roleID int) (io.ReadSeekCloser, string, error) {
	e, err := s.Get(ctx, id, roleID)
	if err != nil {
		return nil, "", err
	}
	if e.PhotoKey == "" {
		return nil, "", ErrNotFound
	}
	r, _, err := s.store.Open(ctx, e.PhotoKey)
	if err != nil {
		return nil, "", ErrNotFound
	}
	ct := "image/jpeg"
	if strings.HasSuffix(e.PhotoKey, ".png") {
		ct = "image/png"
	}
	return r, ct, nil
}

// ─── Шаблоны ────────────────────────────────────────────────────────────────

var hrTemplateKinds = map[string]bool{
	"labor_contract": true, "nda": true, "non_compete": true,
	"liability": true, "job_description": true, "other": true,
}

func (s *EmployeeService) ListTemplates(ctx context.Context, roleID int) ([]*models.HRTemplate, error) {
	if !CanManageEmployees(roleID) {
		return nil, ErrForbidden
	}
	return s.repo.ListTemplates(ctx)
}

const hrTemplateMaxBytes = 20 << 20

// UploadTemplate — шаблон в формате Word (.docx).
func (s *EmployeeService) UploadTemplate(ctx context.Context, name, kind, originalName string, content []byte, userID, roleID int) (*models.HRTemplate, error) {
	if !CanManageHRTemplates(roleID) {
		return nil, ErrForbidden
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(originalName), filepath.Ext(originalName))
	}
	if name == "" {
		return nil, invalid("Укажите название шаблона")
	}
	if !hrTemplateKinds[kind] {
		kind = "other"
	}
	if !strings.EqualFold(filepath.Ext(originalName), ".docx") {
		return nil, invalid("Шаблон — файл Word в формате .docx")
	}
	if len(content) == 0 || len(content) > hrTemplateMaxBytes {
		return nil, invalid("Файл шаблона пустой или больше 20 МБ")
	}
	// .docx — это zip-архив.
	if !bytes.HasPrefix(content, []byte("PK")) {
		return nil, invalid("Файл не похож на документ Word (.docx)")
	}
	key := fmt.Sprintf("hr/templates/%d_%s.docx", s.now().UnixNano(), safeFileBase(name))
	if err := s.store.Save(ctx, bytes.NewReader(content), key); err != nil {
		return nil, fmt.Errorf("save hr template: %w", err)
	}
	uid := userID
	t := &models.HRTemplate{Name: name, Kind: kind, FileKey: key, OriginalName: filepath.Base(originalName), UploadedBy: &uid}
	id, err := s.repo.CreateTemplate(ctx, t)
	if err != nil {
		_ = s.store.Delete(ctx, key)
		return nil, err
	}
	return s.repo.GetTemplate(ctx, id)
}

func (s *EmployeeService) DeleteTemplate(ctx context.Context, id, roleID int) error {
	if !CanManageHRTemplates(roleID) {
		return ErrForbidden
	}
	return s.repo.DeleteTemplate(ctx, id)
}

// OpenTemplate — скачать исходный Word-шаблон.
func (s *EmployeeService) OpenTemplate(ctx context.Context, id, roleID int) (io.ReadSeekCloser, string, error) {
	if !CanManageEmployees(roleID) {
		return nil, "", ErrForbidden
	}
	t, err := s.repo.GetTemplate(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if t == nil {
		return nil, "", ErrNotFound
	}
	r, _, err := s.store.Open(ctx, t.FileKey)
	if err != nil {
		return nil, "", ErrNotFound
	}
	name := t.OriginalName
	if name == "" {
		name = t.Name + ".docx"
	}
	return r, name, nil
}

var unsafeFileChars = regexp.MustCompile(`[^\p{L}\p{N}_-]+`)

func safeFileBase(s string) string {
	s = unsafeFileChars.ReplaceAllString(strings.TrimSpace(s), "_")
	s = strings.Trim(s, "_")
	if r := []rune(s); len(r) > 60 {
		s = string(r[:60])
	}
	if s == "" {
		s = "doc"
	}
	return s
}

// ─── Кадровые документы ─────────────────────────────────────────────────────

var ruMonthsGenitive = []string{"января", "февраля", "марта", "апреля", "мая", "июня",
	"июля", "августа", "сентября", "октября", "ноября", "декабря"}

func formatRuDate(isoDate string) string {
	t, ok := parseDate(isoDate)
	if !ok {
		return ""
	}
	return t.Format("02.01.2006")
}

func formatSalary(v float64) string {
	whole := int64(math.Round(v))
	s := fmt.Sprintf("%d", whole)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// EmployeePlaceholders — подстановки в шаблон: данные сотрудника и дата
// документа. Набор совпадает со списком на странице шаблонов.
func (s *EmployeeService) EmployeePlaceholders(e *models.Employee) map[string]string {
	now := s.now().In(s.displayZone)
	return map[string]string{
		"FULL_NAME":     e.FullName,
		"IIN":           e.IIN,
		"BIRTH_DATE":    formatRuDate(e.BirthDate),
		"ADDRESS":       e.Address,
		"PHONE":         e.Phone,
		"EMAIL":         e.Email,
		"POSITION":      e.Position,
		"SALARY":        formatSalary(e.Salary),
		"SALARY_WORDS":  amountToRuWords(int64(math.Round(e.Salary)), 0),
		"HIRE_DATE":     formatRuDate(e.HiredAt),
		"DOC_DATE":      now.Format("02.01.2006"),
		"DOC_DATE_TEXT": fmt.Sprintf("«%02d» %s %d г.", now.Day(), ruMonthsGenitive[now.Month()-1], now.Year()),
	}
}

func (s *EmployeeService) ListDocuments(ctx context.Context, employeeID, roleID int) ([]*models.EmployeeDocument, error) {
	if _, err := s.Get(ctx, employeeID, roleID); err != nil {
		return nil, err
	}
	return s.repo.ListDocuments(ctx, employeeID)
}

// IssueDocuments — «Выписать документ»: по каждому выбранному шаблону —
// черновик с подставленными данными сотрудника (Word + PDF для предпросмотра
// и подписания).
func (s *EmployeeService) IssueDocuments(ctx context.Context, employeeID int, templateIDs []int, userID, roleID int) ([]*models.EmployeeDocument, error) {
	e, err := s.Get(ctx, employeeID, roleID)
	if err != nil {
		return nil, err
	}
	if len(templateIDs) == 0 {
		return nil, invalid("Выберите хотя бы один шаблон")
	}
	if !s.officeOn {
		return nil, invalid("Конвертация в PDF отключена на сервере (LibreOffice)")
	}
	placeholders := s.EmployeePlaceholders(e)
	out := []*models.EmployeeDocument{}
	seen := map[int]bool{}
	for _, tid := range templateIDs {
		if seen[tid] {
			continue
		}
		seen[tid] = true
		t, err := s.repo.GetTemplate(ctx, tid)
		if err != nil {
			return out, err
		}
		if t == nil {
			return out, invalid(fmt.Sprintf("Шаблон #%d не найден", tid))
		}
		docxKey, pdfKey, err := s.renderTemplate(ctx, t, e, placeholders)
		if err != nil {
			log.Printf("[hr][issue] employee=%d template=%d: %v", employeeID, tid, err)
			return out, invalid(fmt.Sprintf("Не удалось сформировать «%s»: проверьте шаблон", t.Name))
		}
		id, err := s.repo.CreateDocument(ctx, employeeID, t.ID, t.Name, "/"+docxKey, "/"+pdfKey, userID)
		if err != nil {
			return out, err
		}
		d, err := s.repo.GetDocument(ctx, employeeID, id)
		if err != nil {
			return out, err
		}
		out = append(out, d)
	}
	return out, nil
}

// renderTemplate подставляет данные в Word-шаблон и конвертирует в PDF.
// Возвращает ключи файлов в хранилище.
func (s *EmployeeService) renderTemplate(ctx context.Context, t *models.HRTemplate, e *models.Employee, placeholders map[string]string) (string, string, error) {
	tmpDir, err := os.MkdirTemp("", "hrdoc")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(tmpDir)

	src, _, err := s.store.Open(ctx, t.FileKey)
	if err != nil {
		return "", "", fmt.Errorf("open template: %w", err)
	}
	tplPath := filepath.Join(tmpDir, "template.docx")
	f, err := os.Create(tplPath)
	if err != nil {
		_ = src.Close()
		return "", "", err
	}
	_, copyErr := io.Copy(f, src)
	_ = src.Close()
	_ = f.Close()
	if copyErr != nil {
		return "", "", copyErr
	}

	gen := docx.NewDocxGenerator(tmpDir, tmpDir, s.officeOn, s.officeBin)
	rendered, err := gen.RenderDocxToBytes("template.docx", placeholders)
	if err != nil {
		return "", "", fmt.Errorf("render template: %w", err)
	}
	base := fmt.Sprintf("%d_%s", s.now().UnixNano(), safeFileBase(t.Name+"_"+e.FullName))
	docxPath := filepath.Join(tmpDir, base+".docx")
	if err := os.WriteFile(docxPath, rendered, 0o600); err != nil {
		return "", "", err
	}
	pdfPath, err := docx.ConvertOfficeToPDF(ctx, s.officeBin, docxPath, tmpDir)
	if err != nil {
		return "", "", fmt.Errorf("convert to pdf: %w", err)
	}
	prefix := fmt.Sprintf("hr/employees/%d/", e.ID)
	docxKey, pdfKey := prefix+base+".docx", prefix+base+".pdf"
	if err := s.store.Save(ctx, bytes.NewReader(rendered), docxKey); err != nil {
		return "", "", err
	}
	pdfFile, err := os.Open(pdfPath)
	if err != nil {
		return "", "", err
	}
	defer pdfFile.Close()
	if err := s.store.Save(ctx, pdfFile, pdfKey); err != nil {
		_ = s.store.Delete(ctx, docxKey)
		return "", "", err
	}
	return docxKey, pdfKey, nil
}

func (s *EmployeeService) getDocument(ctx context.Context, employeeID int, docID int64, roleID int) (*models.Employee, *models.EmployeeDocument, error) {
	e, err := s.Get(ctx, employeeID, roleID)
	if err != nil {
		return nil, nil, err
	}
	d, err := s.repo.GetDocument(ctx, employeeID, docID)
	if err != nil {
		return nil, nil, err
	}
	if d == nil {
		return nil, nil, ErrNotFound
	}
	return e, d, nil
}

// SendDocument — «Отправить на подписание» (повторно — тоже): сотрудник
// получает ссылку и код по SMS или на email. Подписанный изменить нельзя.
func (s *EmployeeService) SendDocument(ctx context.Context, employeeID int, docID int64, channel string, userID, roleID int) (*models.EmployeeDocument, error) {
	e, d, err := s.getDocument(ctx, employeeID, docID, roleID)
	if err != nil {
		return nil, err
	}
	if d.Status == "signed" {
		return nil, invalid("Документ уже подписан")
	}
	if s.signer == nil {
		return nil, errors.New("signing service unavailable")
	}
	channel = strings.ToLower(strings.TrimSpace(channel))
	to := ""
	switch channel {
	case "sms", "":
		channel = "sms"
		to = normalizePhone(e.Phone)
		if to == "" {
			return nil, invalid("У сотрудника не указан телефон")
		}
		if _, err := s.signer.StartSigningBySMS(ctx, docID, int64(userID), to, e.Email); err != nil {
			log.Printf("[hr][send] doc=%d sms: %v", docID, err)
			return nil, invalid("Не удалось отправить SMS: " + err.Error())
		}
		to = e.Phone
	case "email":
		to = e.Email
		if to == "" {
			return nil, invalid("У сотрудника не указан email")
		}
		if _, err := s.signer.StartSigning(ctx, docID, int64(userID), to); err != nil {
			log.Printf("[hr][send] doc=%d email: %v", docID, err)
			return nil, invalid("Не удалось отправить письмо: " + err.Error())
		}
	default:
		return nil, invalid("Способ отправки: SMS или email")
	}
	if err := s.repo.MarkSent(ctx, docID, channel, to); err != nil {
		return nil, err
	}
	return s.repo.GetDocument(ctx, employeeID, docID)
}

// RevokeDocument — отзыв неподписанного документа.
func (s *EmployeeService) RevokeDocument(ctx context.Context, employeeID int, docID int64, roleID int) (*models.EmployeeDocument, error) {
	_, d, err := s.getDocument(ctx, employeeID, docID, roleID)
	if err != nil {
		return nil, err
	}
	if d.Status == "signed" {
		return nil, invalid("Подписанный документ отозвать нельзя")
	}
	if err := s.repo.Revoke(ctx, docID); err != nil {
		return nil, err
	}
	return s.repo.GetDocument(ctx, employeeID, docID)
}

// OpenDocumentFile — PDF документа для предпросмотра; signed — подписанный
// экземпляр с листом подписания; docx — Word-версия.
func (s *EmployeeService) OpenDocumentFile(ctx context.Context, employeeID int, docID int64, variant string, roleID int) (io.ReadSeekCloser, string, string, error) {
	e, d, err := s.getDocument(ctx, employeeID, docID, roleID)
	if err != nil {
		return nil, "", "", err
	}
	rel, ct, ext := d.FilePathPdf, "application/pdf", ".pdf"
	switch variant {
	case "signed":
		rel = extractSignedPDFPath(d.SignMetadata)
		if rel == "" {
			return nil, "", "", ErrNotFound
		}
	case "docx":
		rel, ct, ext = d.FilePathDocx, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx"
	}
	if strings.TrimSpace(rel) == "" {
		return nil, "", "", ErrNotFound
	}
	key := documentStorageKey(rel)
	if r, _, err := s.store.Open(ctx, key); err == nil {
		return r, ct, safeFileBase(d.Title+"_"+e.FullName) + ext, nil
	}
	f, err := os.Open(filepath.Join(s.filesRoot, filepath.FromSlash(key)))
	if err != nil {
		return nil, "", "", ErrNotFound
	}
	return f, ct, safeFileBase(d.Title+"_"+e.FullName) + ext, nil
}

// IsEmployeeValidation — ошибка валидации раздела «Сотрудники».
func IsEmployeeValidation(err error) (string, bool) {
	var v *EmployeeValidationError
	if errors.As(err, &v) {
		return v.Msg, true
	}
	return "", false
}
