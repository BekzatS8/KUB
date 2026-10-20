package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"turcompany/internal/models"
)

// Загрузка файла частями. nginx перед сайтом и API пропускает запрос до
// ~50 МБ, Next.js-прокси — до 10 МБ, поэтому большой файл браузер режет на
// куски по DriveChunkSize: каждый кусок — отдельный небольшой запрос, сервер
// дописывает их во временный файл и в конце кладёт файл в хранилище обычным
// путём (Upload). Размер файла ограничен только лимитом хранилища (у
// администратора хранилища лимита нет) и диском.

const (
	// DriveChunkSize — размер куска, который присылает браузер (8 МБ: меньше
	// лимита прокси Next.js в 10 МБ).
	DriveChunkSize int64 = 8 << 20
	// DriveMaxChunkBytes — больше этого кусок не принимаем.
	DriveMaxChunkBytes int64 = 16 << 20
	// driveUploadTTL — незавершённая загрузка живёт сутки, потом удаляется.
	driveUploadTTL = 24 * time.Hour
)

var (
	ErrDriveUploadNotFound   = errors.New("drive: upload session not found")
	ErrDriveUploadOffset     = errors.New("drive: chunk offset mismatch")
	ErrDriveUploadIncomplete = errors.New("drive: upload is incomplete")
)

type driveUpload struct {
	mu          sync.Mutex
	id          string
	userID      int
	roleID      int
	parentID    *int64
	name        string
	size        int64
	contentType string
	path        string
	received    int64
	updatedAt   time.Time
}

type driveUploads struct {
	mu   sync.Mutex
	byID map[string]*driveUpload
}

// DriveUploadSession — ответ на начало загрузки.
type DriveUploadSession struct {
	UploadID  string `json:"upload_id"`
	ChunkSize int64  `json:"chunk_size"`
	Received  int64  `json:"received"`
}

func (s *DriveService) uploadsDir() string {
	if s.cfg.TempDir != "" {
		return s.cfg.TempDir
	}
	return filepath.Join(os.TempDir(), "kub-drive-uploads")
}

func (s *DriveService) uploadSessions() *driveUploads {
	s.uploadsOnce.Do(func() {
		s.uploads = &driveUploads{byID: map[string]*driveUpload{}}
	})
	return s.uploads
}

// checkUploadSize — у администратора хранилища лимита на размер файла нет.
func (s *DriveService) checkUploadSize(actor DriveActor, size int64) error {
	if size < 0 {
		return ErrDriveTooLarge
	}
	if !actor.canManage() && size > s.cfg.MaxUploadBytes {
		return ErrDriveTooLarge
	}
	return nil
}

// StartUpload начинает загрузку частями: права и размер проверяются сразу,
// чтобы не гонять гигабайты, которые потом не примут.
func (s *DriveService) StartUpload(ctx context.Context, actor DriveActor, parentID *int64, filename string, size int64, contentType string) (*DriveUploadSession, error) {
	if err := s.canWriteInto(ctx, actor, parentID); err != nil {
		return nil, err
	}
	if err := s.checkUploadSize(actor, size); err != nil {
		return nil, err
	}
	if err := s.ensureFolder(ctx, parentID); err != nil {
		return nil, err
	}
	dir := s.uploadsDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("drive upload dir: %w", err)
	}
	s.cleanupUploads()

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	id := hex.EncodeToString(b)
	path := filepath.Join(dir, id+".part")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, fmt.Errorf("drive upload temp file: %w", err)
	}
	_ = f.Close()

	up := &driveUpload{
		id: id, userID: actor.UserID, roleID: actor.RoleID, parentID: parentID,
		name: filename, size: size, contentType: contentType, path: path, updatedAt: s.now(),
	}
	reg := s.uploadSessions()
	reg.mu.Lock()
	reg.byID[id] = up
	reg.mu.Unlock()
	return &DriveUploadSession{UploadID: id, ChunkSize: DriveChunkSize}, nil
}

func (s *DriveService) upload(id string, actor DriveActor) (*driveUpload, error) {
	reg := s.uploadSessions()
	reg.mu.Lock()
	up, ok := reg.byID[id]
	reg.mu.Unlock()
	if !ok || up.userID != actor.UserID {
		return nil, ErrDriveUploadNotFound
	}
	return up, nil
}

// WriteChunk дописывает кусок с позиции offset. Повтор уже принятого куска
// (сеть оборвалась после записи) — не ошибка: отвечаем, сколько уже есть.
func (s *DriveService) WriteChunk(actor DriveActor, id string, offset int64, r io.Reader) (int64, error) {
	up, err := s.upload(id, actor)
	if err != nil {
		return 0, err
	}
	up.mu.Lock()
	defer up.mu.Unlock()
	if offset < up.received {
		_, _ = io.Copy(io.Discard, r)
		return up.received, nil
	}
	if offset != up.received {
		return up.received, ErrDriveUploadOffset
	}
	f, err := os.OpenFile(up.path, os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return up.received, fmt.Errorf("drive upload open: %w", err)
	}
	n, copyErr := io.Copy(f, io.LimitReader(r, DriveMaxChunkBytes+1))
	closeErr := f.Close()
	if copyErr == nil && n > DriveMaxChunkBytes {
		copyErr = ErrDriveTooLarge
	}
	if copyErr == nil && up.received+n > up.size {
		copyErr = ErrDriveTooLarge
	}
	if copyErr != nil || closeErr != nil {
		// Недописанный кусок отрезаем — повтор начнётся с той же позиции.
		_ = os.Truncate(up.path, up.received)
		if copyErr == nil {
			copyErr = closeErr
		}
		return up.received, copyErr
	}
	up.received += n
	up.updatedAt = s.now()
	return up.received, nil
}

// CompleteUpload кладёт собранный файл в хранилище и удаляет временный.
func (s *DriveService) CompleteUpload(ctx context.Context, actor DriveActor, id string) (*models.DriveNode, error) {
	up, err := s.upload(id, actor)
	if err != nil {
		return nil, err
	}
	up.mu.Lock()
	defer up.mu.Unlock()
	if up.received != up.size {
		return nil, ErrDriveUploadIncomplete
	}
	f, err := os.Open(up.path)
	if err != nil {
		return nil, fmt.Errorf("drive upload reopen: %w", err)
	}
	node, err := s.Upload(ctx, actor, up.parentID, up.name, up.size, up.contentType, f)
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	s.dropUpload(up.id)
	return node, nil
}

// CancelUpload — пользователь отменил загрузку.
func (s *DriveService) CancelUpload(actor DriveActor, id string) error {
	if _, err := s.upload(id, actor); err != nil {
		return err
	}
	s.dropUpload(id)
	return nil
}

func (s *DriveService) dropUpload(id string) {
	reg := s.uploadSessions()
	reg.mu.Lock()
	up, ok := reg.byID[id]
	delete(reg.byID, id)
	reg.mu.Unlock()
	if ok {
		if err := os.Remove(up.path); err != nil && !os.IsNotExist(err) {
			log.Printf("[drive] remove upload temp %s: %v", up.path, err)
		}
	}
}

// cleanupUploads удаляет брошенные загрузки старше суток — и из памяти, и
// оставшиеся после перезапуска файлы *.part.
func (s *DriveService) cleanupUploads() {
	deadline := s.now().Add(-driveUploadTTL)
	reg := s.uploadSessions()
	reg.mu.Lock()
	stale := []string{}
	for id, up := range reg.byID {
		if up.updatedAt.Before(deadline) {
			stale = append(stale, id)
		}
	}
	reg.mu.Unlock()
	for _, id := range stale {
		s.dropUpload(id)
	}
	entries, err := os.ReadDir(s.uploadsDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".part" {
			continue
		}
		info, err := e.Info()
		if err == nil && info.ModTime().Before(deadline) {
			_ = os.Remove(filepath.Join(s.uploadsDir(), e.Name()))
		}
	}
}
