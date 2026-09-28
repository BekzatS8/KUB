package services

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"gopkg.in/gomail.v2"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/storage"
)

// Операции над несколькими элементами хранилища: переместить, копировать,
// свойства и отправить (через мессенджер CRM или на почту).

var (
	ErrDriveMoveIntoSelf  = errors.New("drive: folder cannot be moved or copied into itself")
	ErrDriveNothingToDo   = errors.New("drive: nothing selected")
	ErrDriveSendFolder    = errors.New("drive: only files can be sent")
	ErrDriveSendChannel   = errors.New("drive: unknown send channel")
	ErrDriveSendRecipient = errors.New("drive: recipient is required")
	ErrDriveSendEmail     = errors.New("drive: invalid email")
	ErrDriveMailTooLarge  = errors.New("drive: attachments are too large for email")
	ErrDriveSendDisabled  = errors.New("drive: sending channel is not configured")
	ErrDriveNoPublicURL   = errors.New("drive: public API url is not configured")
	ErrDriveSendFailed    = errors.New("drive: provider rejected the message")
)

const (
	// driveSendLinkTTL — ссылка, по которой мессенджер скачивает файл. С
	// запасом: провайдер может повторить загрузку, если первая не удалась.
	driveSendLinkTTL = 24 * time.Hour
	// driveMailMaxBytes — вложения больше обычно не принимают почтовые серверы.
	driveMailMaxBytes = 20 << 20
	driveMaxBatch     = 200
)

// DriveMessenger — отправка файла клиенту через мессенджер CRM (Wazzup).
type DriveMessenger interface {
	SendMessageText(ctx context.Context, userID int, chatID, transport, channelID, text string) error
	SendFileURL(ctx context.Context, userID int, chatID, transport, channelID, fileURL, fileName, mimeType string, size int64) error
}

// DriveMailFile — вложение письма; Open открывает содержимое по требованию.
type DriveMailFile struct {
	Name        string
	ContentType string
	Open        func() (io.ReadCloser, error)
}

// DriveMailer — отправка файлов на почту.
type DriveMailer interface {
	SendFiles(to, subject, htmlBody string, files []DriveMailFile) error
}

func (s *DriveService) SetMessenger(m DriveMessenger) { s.messenger = m }
func (s *DriveService) SetMailer(m DriveMailer)       { s.mailer = m }

func normalizeDriveIDs(ids []int64) ([]int64, error) {
	seen := make(map[int64]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil, ErrDriveNothingToDo
	}
	if len(out) > driveMaxBatch {
		return nil, fmt.Errorf("%w: не больше %d элементов за раз", ErrDriveNothingToDo, driveMaxBatch)
	}
	return out, nil
}

// checkTarget — папку нельзя положить в неё саму или в её подпапку.
func (s *DriveService) checkTarget(ctx context.Context, node *models.DriveNode, target *int64) error {
	if target == nil || !node.IsFolder() {
		return nil
	}
	within, err := s.repo.IsWithin(ctx, *target, node.ID)
	if err != nil {
		return err
	}
	if within {
		return ErrDriveMoveIntoSelf
	}
	return nil
}

func sameParent(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// Move переносит элементы в папку target (nil — корень). При совпадении
// имени элемент получает номер: «Договор (2).pdf», как при загрузке.
func (s *DriveService) Move(ctx context.Context, actor DriveActor, ids []int64, target *int64) (int, error) {
	ids, err := normalizeDriveIDs(ids)
	if err != nil {
		return 0, err
	}
	if err := s.ensureFolder(ctx, target); err != nil {
		return 0, err
	}
	if err := s.canWriteInto(ctx, actor, target); err != nil {
		return 0, err
	}
	moved := 0
	for _, id := range ids {
		node, err := s.getNode(ctx, id)
		if err != nil {
			return moved, err
		}
		if err := s.canModify(ctx, actor, node); err != nil {
			return moved, err
		}
		if sameParent(node.ParentID, target) {
			continue
		}
		if err := s.checkTarget(ctx, node, target); err != nil {
			return moved, err
		}
		for attempt := 1; attempt <= 50; attempt++ {
			err = s.repo.MoveNode(ctx, id, target, numberedDriveName(node.Name, attempt))
			if !errors.Is(err, repositories.ErrDriveNameTaken) {
				break
			}
		}
		if err != nil {
			return moved, mapDriveRepoErr(err)
		}
		moved++
	}
	return moved, nil
}

// Copy копирует элементы в папку target со всем содержимым: у файлов
// появляются собственные объекты в хранилище. Доступы не копируются.
func (s *DriveService) Copy(ctx context.Context, actor DriveActor, ids []int64, target *int64) (int, error) {
	ids, err := normalizeDriveIDs(ids)
	if err != nil {
		return 0, err
	}
	if err := s.ensureFolder(ctx, target); err != nil {
		return 0, err
	}
	if err := s.canWriteInto(ctx, actor, target); err != nil {
		return 0, err
	}
	copied := 0
	for _, id := range ids {
		node, err := s.getNode(ctx, id)
		if err != nil {
			return copied, err
		}
		// Копировать можно то, что видишь.
		if err := s.ensureAccess(ctx, actor, id); err != nil {
			return copied, err
		}
		if err := s.checkTarget(ctx, node, target); err != nil {
			return copied, err
		}
		subtree, err := s.repo.Subtree(ctx, id)
		if err != nil {
			return copied, err
		}
		newIDs := make(map[int64]int64, len(subtree))
		for _, src := range subtree {
			parent := target
			if src.Depth > 0 {
				p := newIDs[*src.ParentID]
				parent = &p
			}
			dst := &models.DriveNode{ParentID: parent, Kind: src.Kind, SizeBytes: src.SizeBytes, MimeType: src.MimeType, CreatedBy: &actor.UserID}
			if src.Kind == models.DriveKindFile {
				key, err := s.copyObject(ctx, src)
				if err != nil {
					return copied, err
				}
				dst.StorageKey = key
			}
			// Имя с номером нужно только корню копии: внутри новой папки
			// конфликтов нет.
			for attempt := 1; attempt <= 50; attempt++ {
				dst.Name = src.Name
				if src.Depth == 0 {
					dst.Name = numberedDriveName(src.Name, attempt)
				}
				err = s.repo.CreateNode(ctx, dst)
				if src.Depth > 0 || !errors.Is(err, repositories.ErrDriveNameTaken) {
					break
				}
			}
			if err != nil {
				if dst.StorageKey != "" {
					_ = s.store.Delete(context.Background(), dst.StorageKey)
				}
				return copied, mapDriveRepoErr(err)
			}
			newIDs[src.ID] = dst.ID
		}
		copied++
	}
	return copied, nil
}

func (s *DriveService) copyObject(ctx context.Context, src repositories.DriveSubtreeNode) (string, error) {
	key, err := newDriveObjectKey(s.now(), src.Name)
	if err != nil {
		return "", err
	}
	rc, size, err := s.store.Open(ctx, src.StorageKey)
	if err != nil {
		return "", fmt.Errorf("drive copy open %s: %w", src.StorageKey, err)
	}
	defer rc.Close()
	if err := storage.SaveWithSize(ctx, s.store, rc, key, size, src.MimeType); err != nil {
		return "", fmt.Errorf("drive copy save: %w", err)
	}
	return key, nil
}

// DriveProperties — окно «Свойства».
type DriveProperties struct {
	Node *models.DriveNode `json:"node"`
	// Path — путь от корня (у получателя доступа — от открытой ему папки).
	Path []models.DriveBreadcrumb `json:"path"`
	// Для папки — содержимое всех уровней.
	TotalBytes *int64 `json:"total_bytes,omitempty"`
	Files      *int64 `json:"files,omitempty"`
	Folders    *int64 `json:"folders,omitempty"`
}

func (s *DriveService) Properties(ctx context.Context, actor DriveActor, id int64) (*DriveProperties, error) {
	node, err := s.getNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.ensureAccess(ctx, actor, id); err != nil {
		return nil, err
	}
	manage := actor.canManage()
	props := &DriveProperties{Node: s.decorateOne(*node, manage)}
	if node.ParentID != nil {
		if props.Path, err = s.breadcrumbs(ctx, actor, *node.ParentID, manage); err != nil {
			return nil, err
		}
	}
	if props.Path == nil {
		props.Path = []models.DriveBreadcrumb{}
	}
	if node.IsFolder() {
		st, err := s.repo.FolderStats(ctx, id)
		if err != nil {
			return nil, err
		}
		props.TotalBytes, props.Files, props.Folders = &st.Bytes, &st.Files, &st.Folders
	}
	return props, nil
}

// DriveSendRequest — «Отправить»: файлы клиенту в мессенджер или на почту.
type DriveSendRequest struct {
	IDs []int64
	// Channel — whatsapp | telegram | instagram | email.
	Channel string
	// To — телефон / username (мессенджер) или адрес почты.
	To string
	// ChannelID — номер Wazzup, с которого писать (пусто — подберётся).
	ChannelID string
	Text      string
	Subject   string
	// PublicBaseURL — адрес API, известный фронту; используется, если на
	// сервере не задан API_PUBLIC_URL.
	PublicBaseURL string
}

type DriveSendResult struct {
	Sent int `json:"sent"`
}

func (s *DriveService) Send(ctx context.Context, actor DriveActor, req DriveSendRequest) (*DriveSendResult, error) {
	ids, err := normalizeDriveIDs(req.IDs)
	if err != nil {
		return nil, err
	}
	files := make([]*models.DriveNode, 0, len(ids))
	for _, id := range ids {
		node, err := s.getNode(ctx, id)
		if err != nil {
			return nil, err
		}
		if err := s.ensureAccess(ctx, actor, id); err != nil {
			return nil, err
		}
		if node.IsFolder() {
			return nil, ErrDriveSendFolder
		}
		files = append(files, node)
	}
	to := strings.TrimSpace(req.To)
	if to == "" {
		return nil, ErrDriveSendRecipient
	}
	channel := strings.ToLower(strings.TrimSpace(req.Channel))
	switch channel {
	case "email":
		return s.sendByEmail(files, to, req)
	case "whatsapp", "telegram", "instagram":
		return s.sendByMessenger(ctx, actor, files, channel, to, req)
	default:
		return nil, ErrDriveSendChannel
	}
}

func (s *DriveService) publicBase(fromClient string) (string, error) {
	for _, candidate := range []string{s.cfg.PublicAPIURL, fromClient} {
		u, err := url.Parse(strings.TrimSpace(candidate))
		if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" &&
			!strings.Contains(u.Host, "localhost") && !strings.Contains(u.Host, "example.com") {
			return strings.TrimRight(u.Scheme+"://"+u.Host+u.Path, "/"), nil
		}
	}
	return "", ErrDriveNoPublicURL
}

func (s *DriveService) sendByMessenger(ctx context.Context, actor DriveActor, files []*models.DriveNode, transport, to string, req DriveSendRequest) (*DriveSendResult, error) {
	if s.messenger == nil {
		return nil, ErrDriveSendDisabled
	}
	base, err := s.publicBase(req.PublicBaseURL)
	if err != nil {
		return nil, err
	}
	chatID := to
	if transport == "whatsapp" {
		chatID = digitsOnly(to)
	} else {
		chatID = strings.TrimPrefix(to, "@")
	}
	if chatID == "" {
		return nil, ErrDriveSendRecipient
	}
	res := &DriveSendResult{}
	if text := strings.TrimSpace(req.Text); text != "" {
		if err := s.messenger.SendMessageText(ctx, actor.UserID, chatID, transport, req.ChannelID, text); err != nil {
			return res, fmt.Errorf("%w: %v", ErrDriveSendFailed, err)
		}
	}
	expires := s.now().Add(driveSendLinkTTL)
	for _, f := range files {
		token := s.signLink(driveLinkClaims{NodeID: f.ID, UserID: actor.UserID, Expires: expires.Unix(), Variant: DriveVariantOriginal})
		link := base + "/api/v1/drive/raw/" + token
		if err := s.messenger.SendFileURL(ctx, actor.UserID, chatID, transport, req.ChannelID, link, f.Name, f.MimeType, f.SizeBytes); err != nil {
			return res, fmt.Errorf("%w: %v", ErrDriveSendFailed, err)
		}
		res.Sent++
	}
	log.Printf("[drive] sent files=%d via=%s by_user=%d", res.Sent, transport, actor.UserID)
	return res, nil
}

func (s *DriveService) sendByEmail(files []*models.DriveNode, to string, req DriveSendRequest) (*DriveSendResult, error) {
	if s.mailer == nil {
		return nil, ErrDriveSendDisabled
	}
	addr, err := mail.ParseAddress(to)
	if err != nil {
		return nil, ErrDriveSendEmail
	}
	var total int64
	attachments := make([]DriveMailFile, 0, len(files))
	for _, f := range files {
		total += f.SizeBytes
		key := f.StorageKey
		attachments = append(attachments, DriveMailFile{
			Name:        f.Name,
			ContentType: f.MimeType,
			Open: func() (io.ReadCloser, error) {
				rc, _, err := s.store.Open(context.Background(), key)
				return rc, err
			},
		})
	}
	if total > driveMailMaxBytes {
		return nil, ErrDriveMailTooLarge
	}
	subject := strings.TrimSpace(req.Subject)
	if subject == "" {
		subject = "Файлы"
		if len(files) == 1 {
			subject = files[0].Name
		}
	}
	body := "<p>Файлы во вложении.</p>"
	if text := strings.TrimSpace(req.Text); text != "" {
		body = "<p>" + strings.ReplaceAll(html.EscapeString(text), "\n", "<br>") + "</p>"
	}
	if err := s.mailer.SendFiles(addr.Address, subject, body, attachments); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDriveSendFailed, err)
	}
	return &DriveSendResult{Sent: len(files)}, nil
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SendFiles — письмо с вложениями из хранилища. Содержимое читается при
// отправке, а не загружается в память заранее.
func (s *emailService) SendFiles(to, subject, htmlBody string, files []DriveMailFile) error {
	m := gomail.NewMessage()
	setFromHeader(m, s.from, s.fromName)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", htmlBody)
	for _, f := range files {
		open := f.Open
		settings := []gomail.FileSetting{
			gomail.SetCopyFunc(func(w io.Writer) error {
				rc, err := open()
				if err != nil {
					return err
				}
				defer rc.Close()
				_, err = io.Copy(w, rc)
				return err
			}),
		}
		if f.ContentType != "" {
			settings = append(settings, gomail.SetHeader(map[string][]string{"Content-Type": {f.ContentType}}))
		}
		m.Attach(f.Name, settings...)
	}
	if err := s.dialer.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send files email: %w", err)
	}
	return nil
}
