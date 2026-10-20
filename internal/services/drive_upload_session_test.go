package services

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"turcompany/internal/authz"
)

// newTestUpload регистрирует загрузку в обход StartUpload (без базы).
func newTestUpload(t *testing.T, s *DriveService, actor DriveActor, size int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "u.part")
	if err := os.WriteFile(path, nil, 0o640); err != nil {
		t.Fatal(err)
	}
	reg := s.uploadSessions()
	reg.byID["u1"] = &driveUpload{id: "u1", userID: actor.UserID, roleID: actor.RoleID, name: "big.pdf", size: size, path: path, updatedAt: s.now()}
	return path
}

func TestDriveChunksAppendInOrder(t *testing.T) {
	s := NewDriveService(nil, nil, DriveConfig{})
	actor := DriveActor{UserID: 7, RoleID: authz.RoleSystemAdmin}
	path := newTestUpload(t, s, actor, 10)

	if got, err := s.WriteChunk(actor, "u1", 0, bytes.NewReader([]byte("hello"))); err != nil || got != 5 {
		t.Fatalf("first chunk: got %d, %v", got, err)
	}
	// Повтор того же куска после обрыва сети — не ошибка.
	if got, err := s.WriteChunk(actor, "u1", 0, bytes.NewReader([]byte("hello"))); err != nil || got != 5 {
		t.Fatalf("retried chunk must be accepted, got %d, %v", got, err)
	}
	// Кусок не по порядку — отказ с текущей позицией.
	if got, err := s.WriteChunk(actor, "u1", 7, bytes.NewReader([]byte("xx"))); !errors.Is(err, ErrDriveUploadOffset) || got != 5 {
		t.Fatalf("out-of-order chunk: got %d, %v", got, err)
	}
	// Больше заявленного размера — отказ, лишнее не остаётся в файле.
	if _, err := s.WriteChunk(actor, "u1", 5, bytes.NewReader([]byte("world!!"))); !errors.Is(err, ErrDriveTooLarge) {
		t.Fatalf("overflow must be rejected, got %v", err)
	}
	if got, err := s.WriteChunk(actor, "u1", 5, bytes.NewReader([]byte("world"))); err != nil || got != 10 {
		t.Fatalf("last chunk: got %d, %v", got, err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "helloworld" {
		t.Fatalf("assembled file %q", data)
	}

	// Чужая загрузка не видна.
	if _, err := s.WriteChunk(DriveActor{UserID: 8}, "u1", 10, bytes.NewReader(nil)); !errors.Is(err, ErrDriveUploadNotFound) {
		t.Fatalf("other user's upload must be hidden, got %v", err)
	}
	if err := s.CancelUpload(actor, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cancelled upload must remove temp file, stat err=%v", err)
	}
}

// У администратора хранилища лимита на размер нет, у остальных — есть.
func TestDriveUploadSizeLimitSkipsAdmin(t *testing.T) {
	s := NewDriveService(nil, nil, DriveConfig{MaxUploadBytes: 100})
	if err := s.checkUploadSize(DriveActor{UserID: 1, RoleID: authz.RoleSystemAdmin}, 50<<30); err != nil {
		t.Fatalf("admin must have no size limit, got %v", err)
	}
	if err := s.checkUploadSize(DriveActor{UserID: 2, RoleID: authz.RoleSales}, 101); !errors.Is(err, ErrDriveTooLarge) {
		t.Fatalf("staff must keep the limit, got %v", err)
	}
}
