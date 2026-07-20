package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// nopSeekCloser оборачивает bytes.Reader как io.ReadSeekCloser.
type nopSeekCloser struct{ *bytes.Reader }

func (nopSeekCloser) Close() error { return nil }

// fakeStore имитирует S3: отдаёт содержимое по ключу, если он положен.
type fakeStore struct {
	files map[string][]byte
	opens []string // какие ключи запрашивали
}

func (f *fakeStore) Save(context.Context, io.Reader, string) error { return nil }
func (f *fakeStore) Delete(context.Context, string) error          { return nil }
func (f *fakeStore) Open(_ context.Context, key string) (io.ReadSeekCloser, int64, error) {
	f.opens = append(f.opens, key)
	b, ok := f.files[key]
	if !ok {
		return nil, 0, errors.New("not found in store")
	}
	return nopSeekCloser{bytes.NewReader(b)}, int64(len(b)), nil
}

// На S3-деплое документ читается из хранилища, а не с локального диска —
// это и был баг «Failed to confirm signing».
func TestOpenDocumentContentFromStore(t *testing.T) {
	store := &fakeStore{files: map[string][]byte{
		"docx/contract.pdf": []byte("%PDF-from-s3"),
	}}
	svc := &DocumentSigningConfirmationService{filesRoot: "/nonexistent"}
	svc.SetStore(store)

	// путь приходит как в БД: с префиксом files/ и ведущим слэшем
	r, err := svc.openDocumentContent("/files/docx/contract.pdf")
	if err != nil {
		t.Fatalf("open from store: %v", err)
	}
	defer r.Close()
	data, _ := io.ReadAll(r)
	if string(data) != "%PDF-from-s3" {
		t.Fatalf("получено не из S3: %q", data)
	}
	if len(store.opens) == 0 || store.opens[0] != "docx/contract.pdf" {
		t.Fatalf("ключ хранилища нормализован неверно: %v", store.opens)
	}
	if !svc.documentContentExists("/files/docx/contract.pdf") {
		t.Fatal("documentContentExists должен видеть файл в S3")
	}
}

// Если файла нет в S3 (ещё не выгружен) — берём с локального диска.
func TestOpenDocumentContentFallsBackToLocal(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docx", "local.pdf"), []byte("%PDF-local"), 0o644); err != nil {
		t.Fatal(err)
	}
	// store пуст → Open вернёт ошибку → фоллбэк на диск
	store := &fakeStore{files: map[string][]byte{}}
	svc := &DocumentSigningConfirmationService{filesRoot: dir}
	svc.SetStore(store)

	r, err := svc.openDocumentContent("files/docx/local.pdf")
	if err != nil {
		t.Fatalf("fallback to local: %v", err)
	}
	defer r.Close()
	data, _ := io.ReadAll(r)
	if string(data) != "%PDF-local" {
		t.Fatalf("получено не с диска: %q", data)
	}
}

// Без хранилища (local-only режим) читаем прямо с диска.
func TestOpenDocumentContentLocalOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "doc.pdf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := &DocumentSigningConfirmationService{filesRoot: dir}
	if !svc.documentContentExists("doc.pdf") {
		t.Fatal("local-only: файл должен находиться")
	}
	if svc.documentContentExists("missing.pdf") {
		t.Fatal("несуществующий файл не должен находиться")
	}
}
