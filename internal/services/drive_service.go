package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"turcompany/internal/authz"
	"turcompany/internal/docx"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/storage"
)

var (
	ErrDriveForbidden          = errors.New("drive: forbidden")
	ErrDriveNotFound           = errors.New("drive: not found")
	ErrDriveNameTaken          = errors.New("drive: name already exists in this folder")
	ErrDriveBadName            = errors.New("drive: invalid name")
	ErrDriveNotFolder          = errors.New("drive: parent is not a folder")
	ErrDriveNotFile            = errors.New("drive: node is not a file")
	ErrDriveTooLarge           = errors.New("drive: file is too large")
	ErrDriveLinkInvalid        = errors.New("drive: link is invalid")
	ErrDriveLinkExpired        = errors.New("drive: link has expired")
	ErrDrivePreviewUnavailable = errors.New("drive: preview is not available for this file")
	ErrDriveBadExpiry          = errors.New("drive: expiry must be in the future")
	ErrDriveNoUsers            = errors.New("drive: no users selected")
)

const (
	// ActionDriveManage — создавать папки, загружать, переименовывать, удалять
	// и выдавать доступы. По умолчанию только у администратора.
	ActionDriveManage = "drive.manage"

	driveKeyPrefix     = "drive/"
	drivePreviewPrefix = "drive-previews/"
	driveMaxNameRunes  = 255
	// Ссылка на содержимое живёт час: этого хватает досмотреть видео, а
	// утёкшая ссылка перестаёт работать быстро.
	driveLinkTTL = time.Hour

	DriveVariantOriginal = "original"
	DriveVariantPDF      = "pdf"
)

// DriveActor — кто выполняет действие.
type DriveActor struct {
	UserID int
	RoleID int
}

func (a DriveActor) canManage() bool {
	return authz.Can(authz.UserContext{UserID: a.UserID, RoleID: a.RoleID}, ActionDriveManage, "drive")
}

// DriveListing — содержимое папки для экрана хранилища.
type DriveListing struct {
	Folder      *models.DriveNode        `json:"folder"`
	Items       []models.DriveNode       `json:"items"`
	Breadcrumbs []models.DriveBreadcrumb `json:"breadcrumbs"`
	CanManage   bool                     `json:"can_manage"`
	// SharedView — пользователь смотрит не всё хранилище, а только выданное ему.
	SharedView bool `json:"shared_view"`
	// Объём хранилища — только администратору (у остальных nil и поле не
	// отдаётся). Указатель, а не omitempty по значению: иначе у пустого
	// хранилища пропадал бы честный «0 Б».
	TotalBytes *int64 `json:"total_bytes,omitempty"`
	TotalFiles *int64 `json:"total_files,omitempty"`
}

// DriveLink — временная ссылка на содержимое файла.
type DriveLink struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// DriveContent — поток для отдачи файла клиенту.
type DriveContent struct {
	Reader      io.ReadSeekCloser
	Size        int64
	Name        string
	ContentType string
	Inline      bool
	ModTime     time.Time
}

type DriveConfig struct {
	MaxUploadBytes int64
	// LinkSecret — ключ подписи временных ссылок (производный от JWT-секрета).
	LinkSecret []byte
	// Office — предпросмотр doc/xls/ppt через конвертацию LibreOffice в PDF.
	OfficeEnabled bool
	OfficeBinary  string
}

type DriveService struct {
	repo  repositories.DriveRepository
	store storage.Storage
	cfg   DriveConfig
	now   func() time.Time

	// Один документ не конвертируем параллельно дважды: второй запрос ждёт и
	// берёт готовый PDF из кэша.
	convertLocks sync.Map // map[int64]*sync.Mutex
}

func NewDriveService(repo repositories.DriveRepository, store storage.Storage, cfg DriveConfig) *DriveService {
	if cfg.MaxUploadBytes <= 0 {
		cfg.MaxUploadBytes = 2 << 30 // 2 ГБ
	}
	return &DriveService{repo: repo, store: store, cfg: cfg, now: time.Now}
}

func (s *DriveService) MaxUploadBytes() int64 { return s.cfg.MaxUploadBytes }

// ─── Просмотр ────────────────────────────────────────────────────────────────

// List отдаёт содержимое папки. parentID = nil — корень: у администратора это
// корень всего хранилища, у остальных — «Доступные мне».
func (s *DriveService) List(ctx context.Context, actor DriveActor, parentID *int64) (*DriveListing, error) {
	manage := actor.canManage()
	listing := &DriveListing{CanManage: manage, SharedView: !manage, Breadcrumbs: []models.DriveBreadcrumb{}}

	if parentID == nil {
		var (
			items []models.DriveNode
			err   error
		)
		if manage {
			items, err = s.repo.ListChildren(ctx, nil)
		} else {
			items, err = s.repo.SharedRoots(ctx, actor.UserID)
		}
		if err != nil {
			return nil, err
		}
		listing.Items = s.decorate(items, manage)
		if manage {
			totalBytes, totalFiles, err := s.repo.Totals(ctx)
			if err != nil {
				return nil, err
			}
			listing.TotalBytes, listing.TotalFiles = &totalBytes, &totalFiles
		}
		return listing, nil
	}

	folder, err := s.getNode(ctx, *parentID)
	if err != nil {
		return nil, err
	}
	if !folder.IsFolder() {
		return nil, ErrDriveNotFolder
	}
	if err := s.ensureAccess(ctx, actor, folder.ID); err != nil {
		return nil, err
	}
	items, err := s.repo.ListChildren(ctx, &folder.ID)
	if err != nil {
		return nil, err
	}
	crumbs, err := s.breadcrumbs(ctx, actor, folder.ID, manage)
	if err != nil {
		return nil, err
	}
	listing.Folder = s.decorateOne(*folder, manage)
	listing.Items = s.decorate(items, manage)
	listing.Breadcrumbs = crumbs
	return listing, nil
}

// breadcrumbs — путь до папки. Получателю доступа путь показывается начиная с
// папки, которую ему открыли: что лежит выше, он видеть не должен.
func (s *DriveService) breadcrumbs(ctx context.Context, actor DriveActor, nodeID int64, manage bool) ([]models.DriveBreadcrumb, error) {
	chain, err := s.repo.Ancestors(ctx, nodeID, actor.UserID)
	if err != nil {
		return nil, err
	}
	start := 0
	if !manage {
		start = -1
		for i, a := range chain {
			if a.SharedWithUser {
				start = i
				break
			}
		}
		if start < 0 {
			return []models.DriveBreadcrumb{}, nil
		}
	}
	out := make([]models.DriveBreadcrumb, 0, len(chain)-start)
	for _, a := range chain[start:] {
		out = append(out, models.DriveBreadcrumb{ID: a.ID, Name: a.Name})
	}
	return out, nil
}

func (s *DriveService) decorate(items []models.DriveNode, manage bool) []models.DriveNode {
	out := make([]models.DriveNode, 0, len(items))
	for _, n := range items {
		out = append(out, *s.decorateOne(n, manage))
	}
	return out
}

func (s *DriveService) decorateOne(n models.DriveNode, manage bool) *models.DriveNode {
	if !n.IsFolder() {
		n.Preview = DrivePreviewKind(n.Name, n.MimeType, s.cfg.OfficeEnabled)
	}
	if !manage {
		// Кому ещё открыт доступ — информация администратора.
		n.SharesCount = 0
	}
	return &n
}

// ─── Управление ─────────────────────────────────────────────────────────────

func (s *DriveService) CreateFolder(ctx context.Context, actor DriveActor, parentID *int64, name string) (*models.DriveNode, error) {
	if !actor.canManage() {
		return nil, ErrDriveForbidden
	}
	name, err := NormalizeDriveName(name)
	if err != nil {
		return nil, err
	}
	if err := s.ensureFolder(ctx, parentID); err != nil {
		return nil, err
	}
	node := &models.DriveNode{ParentID: parentID, Kind: models.DriveKindFolder, Name: name, CreatedBy: &actor.UserID}
	if err := s.repo.CreateNode(ctx, node); err != nil {
		return nil, mapDriveRepoErr(err)
	}
	return s.reload(ctx, node.ID, true)
}

// Upload кладёт файл в объектное хранилище и регистрирует его в папке.
//
// Сначала объект, потом запись в базе: если запись не удалась, объект
// удаляется — иначе в бакете копились бы файлы, которых никто не видит. При
// совпадении имени файл переименовывается в «имя (2).ext», как в Яндекс Диске,
// а не отклоняется: загрузка пачкой не должна падать из-за одного дубля.
func (s *DriveService) Upload(ctx context.Context, actor DriveActor, parentID *int64, filename string, size int64, headerType string, r io.Reader) (*models.DriveNode, error) {
	if !actor.canManage() {
		return nil, ErrDriveForbidden
	}
	if size > s.cfg.MaxUploadBytes {
		return nil, ErrDriveTooLarge
	}
	name := SanitizeDriveFileName(filename)
	if err := s.ensureFolder(ctx, parentID); err != nil {
		return nil, err
	}
	contentType := DetectDriveContentType(name, headerType)

	key, err := newDriveObjectKey(s.now(), name)
	if err != nil {
		return nil, err
	}
	if err := storage.SaveWithSize(ctx, s.store, r, key, size, contentType); err != nil {
		return nil, fmt.Errorf("drive upload to storage: %w", err)
	}

	node := &models.DriveNode{
		ParentID: parentID, Kind: models.DriveKindFile, StorageKey: key,
		SizeBytes: size, MimeType: contentType, CreatedBy: &actor.UserID,
	}
	for attempt := 1; attempt <= 50; attempt++ {
		node.Name = numberedDriveName(name, attempt)
		err = s.repo.CreateNode(ctx, node)
		if !errors.Is(err, repositories.ErrDriveNameTaken) {
			break
		}
	}
	if err != nil {
		if delErr := s.store.Delete(context.Background(), key); delErr != nil {
			log.Printf("[drive] orphan object after failed insert key=%s err=%v", key, delErr)
		}
		return nil, mapDriveRepoErr(err)
	}
	return s.reload(ctx, node.ID, true)
}

func (s *DriveService) Rename(ctx context.Context, actor DriveActor, id int64, name string) (*models.DriveNode, error) {
	if !actor.canManage() {
		return nil, ErrDriveForbidden
	}
	name, err := NormalizeDriveName(name)
	if err != nil {
		return nil, err
	}
	if err := s.repo.RenameNode(ctx, id, name); err != nil {
		return nil, mapDriveRepoErr(err)
	}
	return s.reload(ctx, id, true)
}

// Delete удаляет файл или папку со всем содержимым. Возвращает, сколько
// файлов ушло из хранилища.
func (s *DriveService) Delete(ctx context.Context, actor DriveActor, id int64) (int, error) {
	if !actor.canManage() {
		return 0, ErrDriveForbidden
	}
	objects, err := s.repo.DeleteNode(ctx, id)
	if err != nil {
		return 0, mapDriveRepoErr(err)
	}
	// Записи уже удалены — объекты чистим «лучшим усилием». Ошибка здесь не
	// должна возвращать удалённое: оставшийся в бакете объект недоступен
	// никому и только занимает место, это видно в логе.
	for _, o := range objects {
		if o.StorageKey != "" {
			if err := s.store.Delete(context.Background(), o.StorageKey); err != nil {
				log.Printf("[drive] delete object failed key=%s err=%v", o.StorageKey, err)
			}
		}
		_ = s.store.Delete(context.Background(), drivePreviewKey(o.NodeID))
	}
	return len(objects), nil
}

// ─── Доступы ────────────────────────────────────────────────────────────────

func (s *DriveService) ListShares(ctx context.Context, actor DriveActor, id int64) ([]models.DriveShare, error) {
	if !actor.canManage() {
		return nil, ErrDriveForbidden
	}
	if _, err := s.getNode(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.ListShares(ctx, id)
}

// Share открывает доступ к файлу или папке. expiresAt = nil — бессрочно.
// Повторная выдача тому же пользователю меняет срок.
func (s *DriveService) Share(ctx context.Context, actor DriveActor, id int64, userIDs []int, expiresAt *time.Time) ([]models.DriveShare, error) {
	if !actor.canManage() {
		return nil, ErrDriveForbidden
	}
	ids := uniquePositiveIDs(userIDs)
	if len(ids) == 0 {
		return nil, ErrDriveNoUsers
	}
	if expiresAt != nil && !expiresAt.After(s.now()) {
		return nil, ErrDriveBadExpiry
	}
	if _, err := s.getNode(ctx, id); err != nil {
		return nil, err
	}
	if err := s.repo.UpsertShares(ctx, id, ids, expiresAt, actor.UserID); err != nil {
		return nil, mapDriveRepoErr(err)
	}
	return s.repo.ListShares(ctx, id)
}

func (s *DriveService) Unshare(ctx context.Context, actor DriveActor, shareID int64) error {
	if !actor.canManage() {
		return ErrDriveForbidden
	}
	return mapDriveRepoErr(s.repo.DeleteShare(ctx, shareID))
}

func (s *DriveService) Users(ctx context.Context, actor DriveActor) ([]models.DriveUser, error) {
	if !actor.canManage() {
		return nil, ErrDriveForbidden
	}
	return s.repo.ListUsers(ctx)
}

// ─── Содержимое и временные ссылки ──────────────────────────────────────────

// Link выдаёт временную ссылку на содержимое файла.
//
// Зачем ссылка, а не отдача по JWT: <img>, <video> и <iframe> не умеют
// передавать заголовок Authorization. Подписанная ссылка позволяет браузеру
// тянуть файл напрямую — с потоковой отдачей и перемоткой видео, без загрузки
// целиком в память вкладки.
func (s *DriveService) Link(ctx context.Context, actor DriveActor, id int64, variant string, inline bool) (*DriveLink, error) {
	node, err := s.getNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if node.IsFolder() {
		return nil, ErrDriveNotFile
	}
	if err := s.ensureAccess(ctx, actor, node.ID); err != nil {
		return nil, err
	}
	if variant != DriveVariantPDF {
		variant = DriveVariantOriginal
	}
	if variant == DriveVariantPDF && DrivePreviewKind(node.Name, node.MimeType, s.cfg.OfficeEnabled) != "office" {
		return nil, ErrDrivePreviewUnavailable
	}
	expires := s.now().Add(driveLinkTTL)
	token := s.signLink(driveLinkClaims{NodeID: node.ID, UserID: actor.UserID, Expires: expires.Unix(), Variant: variant, Inline: inline})
	return &DriveLink{URL: "/api/v1/drive/raw/" + token, ExpiresAt: expires}, nil
}

// Open отдаёт содержимое по временной ссылке. Доступ перепроверяется: если
// его отозвали, ссылка перестаёт работать сразу, не дожидаясь истечения.
func (s *DriveService) Open(ctx context.Context, token string) (*DriveContent, error) {
	claims, err := s.verifyLink(token)
	if err != nil {
		return nil, err
	}
	node, err := s.getNode(ctx, claims.NodeID)
	if err != nil {
		return nil, err
	}
	if node.IsFolder() {
		return nil, ErrDriveNotFile
	}
	isAdmin, err := s.repo.UserHasRole(ctx, claims.UserID, authz.RoleSystemAdmin)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		ok, err := s.repo.CanAccess(ctx, node.ID, claims.UserID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrDriveForbidden
		}
	}

	if claims.Variant == DriveVariantPDF {
		return s.openOfficePreview(ctx, node)
	}
	reader, size, err := s.store.Open(ctx, node.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("drive open object: %w", err)
	}
	return &DriveContent{
		Reader: reader, Size: size, Name: node.Name,
		ContentType: servedContentType(node.Name, node.MimeType, claims.Inline),
		Inline:      claims.Inline, ModTime: node.UpdatedAt,
	}, nil
}

// openOfficePreview отдаёт PDF-версию офисного документа, конвертируя его при
// первом открытии. Готовый PDF кэшируется в хранилище рядом с оригиналом:
// конвертация занимает секунды, и повторять её на каждый просмотр незачем.
func (s *DriveService) openOfficePreview(ctx context.Context, node *models.DriveNode) (*DriveContent, error) {
	if DrivePreviewKind(node.Name, node.MimeType, s.cfg.OfficeEnabled) != "office" {
		return nil, ErrDrivePreviewUnavailable
	}
	previewName := strings.TrimSuffix(node.Name, path.Ext(node.Name)) + ".pdf"
	cached := func() (*DriveContent, bool) {
		reader, size, err := s.store.Open(ctx, drivePreviewKey(node.ID))
		if err != nil {
			return nil, false
		}
		return &DriveContent{Reader: reader, Size: size, Name: previewName,
			ContentType: "application/pdf", Inline: true, ModTime: node.UpdatedAt}, true
	}
	if c, ok := cached(); ok {
		return c, nil
	}

	lockAny, _ := s.convertLocks.LoadOrStore(node.ID, &sync.Mutex{})
	lock := lockAny.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	if c, ok := cached(); ok { // пока ждали, конвертировал другой запрос
		return c, nil
	}

	workDir, err := os.MkdirTemp("", "drive_preview_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workDir)

	// Имя входного файла — служебное: пользовательское может содержать что
	// угодно, а LibreOffice выбирает фильтр по расширению.
	inputPath := filepath.Join(workDir, "input"+strings.ToLower(path.Ext(node.Name)))
	if err := s.copyObjectToFile(ctx, node.StorageKey, inputPath); err != nil {
		return nil, err
	}
	pdfPath, err := docx.ConvertOfficeToPDF(ctx, s.cfg.OfficeBinary, inputPath, filepath.Join(workDir, "out"))
	if err != nil {
		log.Printf("[drive] office preview failed node=%d err=%v", node.ID, err)
		return nil, ErrDrivePreviewUnavailable
	}
	pdf, err := os.Open(pdfPath)
	if err != nil {
		return nil, err
	}
	st, err := pdf.Stat()
	if err != nil {
		_ = pdf.Close()
		return nil, err
	}
	saveErr := storage.SaveWithSize(ctx, s.store, pdf, drivePreviewKey(node.ID), st.Size(), "application/pdf")
	_ = pdf.Close()
	if saveErr != nil {
		return nil, fmt.Errorf("drive cache preview: %w", saveErr)
	}
	if c, ok := cached(); ok {
		return c, nil
	}
	return nil, ErrDrivePreviewUnavailable
}

func (s *DriveService) copyObjectToFile(ctx context.Context, key, dst string) error {
	reader, _, err := s.store.Open(ctx, key)
	if err != nil {
		return fmt.Errorf("drive open object: %w", err)
	}
	defer reader.Close()
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, reader); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// ─── Подпись ссылок ─────────────────────────────────────────────────────────

type driveLinkClaims struct {
	NodeID  int64
	UserID  int
	Expires int64
	Variant string
	Inline  bool
}

func (s *DriveService) signLink(c driveLinkClaims) string {
	inline := "0"
	if c.Inline {
		inline = "1"
	}
	payload := strings.Join([]string{
		strconv.FormatInt(c.NodeID, 10), strconv.Itoa(c.UserID),
		strconv.FormatInt(c.Expires, 10), c.Variant, inline,
	}, ".")
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(payload)) + "." + enc.EncodeToString(s.mac(payload))
}

func (s *DriveService) verifyLink(token string) (*driveLinkClaims, error) {
	enc := base64.RawURLEncoding
	encPayload, encSig, ok := strings.Cut(token, ".")
	if !ok {
		return nil, ErrDriveLinkInvalid
	}
	payloadBytes, err := enc.DecodeString(encPayload)
	if err != nil {
		return nil, ErrDriveLinkInvalid
	}
	sig, err := enc.DecodeString(encSig)
	if err != nil {
		return nil, ErrDriveLinkInvalid
	}
	payload := string(payloadBytes)
	if !hmac.Equal(sig, s.mac(payload)) {
		return nil, ErrDriveLinkInvalid
	}
	parts := strings.Split(payload, ".")
	if len(parts) != 5 {
		return nil, ErrDriveLinkInvalid
	}
	nodeID, err1 := strconv.ParseInt(parts[0], 10, 64)
	userID, err2 := strconv.Atoi(parts[1])
	expires, err3 := strconv.ParseInt(parts[2], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return nil, ErrDriveLinkInvalid
	}
	if s.now().Unix() > expires {
		return nil, ErrDriveLinkExpired
	}
	return &driveLinkClaims{NodeID: nodeID, UserID: userID, Expires: expires, Variant: parts[3], Inline: parts[4] == "1"}, nil
}

func (s *DriveService) mac(payload string) []byte {
	m := hmac.New(sha256.New, s.cfg.LinkSecret)
	m.Write([]byte(payload))
	return m.Sum(nil)
}

// DriveLinkSecret выводит ключ подписи ссылок из JWT-секрета. Отдельный ключ
// (а не сам JWT-секрет) — чтобы подпись ссылки нельзя было выдать за токен.
func DriveLinkSecret(jwtSecret []byte) []byte {
	m := hmac.New(sha256.New, jwtSecret)
	m.Write([]byte("kub-drive-link-v1"))
	return m.Sum(nil)
}

// ─── Вспомогательное ────────────────────────────────────────────────────────

func (s *DriveService) getNode(ctx context.Context, id int64) (*models.DriveNode, error) {
	node, err := s.repo.GetNode(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDriveNotFound
	}
	return node, err
}

func (s *DriveService) reload(ctx context.Context, id int64, manage bool) (*models.DriveNode, error) {
	node, err := s.getNode(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.decorateOne(*node, manage), nil
}

func (s *DriveService) ensureFolder(ctx context.Context, parentID *int64) error {
	if parentID == nil {
		return nil
	}
	parent, err := s.getNode(ctx, *parentID)
	if err != nil {
		return err
	}
	if !parent.IsFolder() {
		return ErrDriveNotFolder
	}
	return nil
}

func (s *DriveService) ensureAccess(ctx context.Context, actor DriveActor, nodeID int64) error {
	if actor.canManage() {
		return nil
	}
	ok, err := s.repo.CanAccess(ctx, nodeID, actor.UserID)
	if err != nil {
		return err
	}
	if !ok {
		// Узел для постороннего неотличим от несуществующего — не подсказываем,
		// что по этому id что-то лежит.
		return ErrDriveNotFound
	}
	return nil
}

func mapDriveRepoErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repositories.ErrDriveNameTaken):
		return ErrDriveNameTaken
	case errors.Is(err, sql.ErrNoRows):
		return ErrDriveNotFound
	default:
		return err
	}
}

func drivePreviewKey(nodeID int64) string {
	return fmt.Sprintf("%s%d.pdf", drivePreviewPrefix, nodeID)
}

// newDriveObjectKey — ключ объекта: drive/2026/09/<случайный id><ext>.
// Имя файла в ключ не попадает: оно меняется при переименовании и может
// содержать что угодно, а ключ должен быть стабильным и безопасным.
func newDriveObjectKey(now time.Time, name string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	ext := strings.ToLower(path.Ext(name))
	if len(ext) > 16 || strings.ContainsAny(ext, `/\`) {
		ext = ""
	}
	return fmt.Sprintf("%s%s/%s%s", driveKeyPrefix, now.UTC().Format("2006/01"), hex.EncodeToString(b), ext), nil
}

// NormalizeDriveName проверяет имя папки или новое имя при переименовании.
func NormalizeDriveName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "", ErrDriveBadName
	}
	if strings.ContainsAny(name, `/\`) || utf8.RuneCountInString(name) > driveMaxNameRunes {
		return "", ErrDriveBadName
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", ErrDriveBadName
		}
	}
	return name, nil
}

// SanitizeDriveFileName приводит имя загружаемого файла к безопасному виду.
// В отличие от NormalizeDriveName не отклоняет, а исправляет: имя приходит из
// браузера, и отказывать в загрузке из-за странного символа незачем.
func SanitizeDriveFileName(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	name = path.Base(name)
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || name == "/" {
		name = "file"
	}
	if utf8.RuneCountInString(name) > driveMaxNameRunes {
		ext := path.Ext(name)
		if utf8.RuneCountInString(ext) > 16 {
			ext = ""
		}
		runes := []rune(strings.TrimSuffix(name, ext))
		name = string(runes[:driveMaxNameRunes-utf8.RuneCountInString(ext)]) + ext
	}
	return name
}

// numberedDriveName: attempt=1 → «отчёт.pdf», 2 → «отчёт (2).pdf».
func numberedDriveName(name string, attempt int) string {
	if attempt <= 1 {
		return name
	}
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if base == "" { // «.env» — расширение и есть имя
		base, ext = name, ""
	}
	return fmt.Sprintf("%s (%d)%s", base, attempt, ext)
}

// driveKnownTypes — типы частых расширений, заданные явно.
//
// mime.TypeByExtension зависит от ОС сервера: на Windows он читает реестр (там
// .csv принадлежит Excel — application/vnd.ms-excel), на Linux —
// /etc/mime.types, которого в контейнере может не быть, а во встроенной
// таблице Go нет даже .csv и .docx. Из-за этого один и тот же файл получал
// разный тип на разных машинах, и CSV вместо показа уходил на скачивание.
var driveKnownTypes = map[string]string{
	".pdf":  "application/pdf",
	".txt":  "text/plain; charset=utf-8",
	".md":   "text/markdown; charset=utf-8",
	".csv":  "text/csv; charset=utf-8",
	".log":  "text/plain; charset=utf-8",
	".json": "application/json",
	".xml":  "application/xml",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
	".heic": "image/heic",
	".svg":  "image/svg+xml",
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".mov":  "video/quicktime",
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".ogg":  "audio/ogg",
	".m4a":  "audio/mp4",
	".doc":  "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xls":  "application/vnd.ms-excel",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".ppt":  "application/vnd.ms-powerpoint",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".odt":  "application/vnd.oasis.opendocument.text",
	".ods":  "application/vnd.oasis.opendocument.spreadsheet",
	".odp":  "application/vnd.oasis.opendocument.presentation",
	".rtf":  "application/rtf",
	".zip":  "application/zip",
	".rar":  "application/vnd.rar",
	".7z":   "application/x-7z-compressed",
}

// DetectDriveContentType определяет тип по расширению, а заголовок браузера
// берёт только как запасной вариант: браузеры нередко шлют
// application/octet-stream даже для PDF, и тогда не работал бы предпросмотр.
func DetectDriveContentType(name, headerType string) string {
	ext := strings.ToLower(path.Ext(name))
	if known, ok := driveKnownTypes[ext]; ok {
		return known
	}
	if byExt := mime.TypeByExtension(ext); byExt != "" {
		return byExt
	}
	if ct := strings.TrimSpace(headerType); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

var driveOfficeExts = map[string]bool{
	".doc": true, ".docx": true, ".odt": true, ".rtf": true,
	".xls": true, ".xlsx": true, ".ods": true,
	".ppt": true, ".pptx": true, ".odp": true,
}

var driveTextExts = map[string]bool{
	".txt": true, ".md": true, ".csv": true, ".log": true, ".json": true,
	".xml": true, ".yaml": true, ".yml": true, ".ini": true, ".conf": true,
}

// DrivePreviewKind — как показать файл без скачивания.
func DrivePreviewKind(name, mimeType string, officeEnabled bool) string {
	ext := strings.ToLower(path.Ext(name))
	// Параметры типа («; charset=utf-8») для классификации не важны.
	mt, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(mimeType)), ";")
	mt = strings.TrimSpace(mt)
	switch {
	case ext == ".pdf" || mt == "application/pdf":
		return "pdf"
	case strings.HasPrefix(mt, "image/") && ext != ".svg" && mt != "image/svg+xml":
		// SVG — это документ со скриптами, а не картинка; его не показываем.
		return "image"
	case strings.HasPrefix(mt, "video/"):
		return "video"
	case strings.HasPrefix(mt, "audio/"):
		return "audio"
	case driveTextExts[ext] || mt == "text/plain":
		return "text"
	case driveOfficeExts[ext]:
		if officeEnabled {
			return "office"
		}
	}
	return ""
}

// servedContentType — с каким Content-Type отдавать файл.
//
// При показе в браузере всё текстовое отдаётся как text/plain в UTF-8: иначе
// HTML или SVG из хранилища исполнились бы как страница на домене API, а CSV
// браузер предложил бы скачать вместо показа.
func servedContentType(name, stored string, inline bool) string {
	if stored == "" {
		stored = "application/octet-stream"
	}
	if !inline {
		return stored
	}
	// Решение «показывать как текст» принимается по тому же правилу, что и
	// предпросмотр в интерфейсе: иначе файл, который интерфейс открыл как
	// текст, мог прийти с «чужим» типом из базы и уйти на скачивание.
	if DrivePreviewKind(name, stored, false) == "text" || isTextualMime(stored) {
		return "text/plain; charset=utf-8"
	}
	return stored
}

// isTextualMime — тип, который браузер отрисовал бы как документ или текст.
// Сравнение точное: «xml» внутри application/vnd.openxmlformats-… (docx, xlsx)
// не должно превращать офисный файл в текст.
func isTextualMime(mt string) bool {
	base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(mt)), ";")
	base = strings.TrimSpace(base)
	switch {
	case strings.HasPrefix(base, "text/"):
		return true
	case base == "application/json" || strings.HasSuffix(base, "+json"):
		return true
	case base == "application/xml" || strings.HasSuffix(base, "+xml"): // в т.ч. image/svg+xml
		return true
	case strings.Contains(base, "javascript") || base == "application/ecmascript":
		return true
	case base == "application/yaml" || base == "application/x-yaml":
		return true
	}
	return false
}

func uniquePositiveIDs(ids []int) []int {
	seen := make(map[int]bool, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
