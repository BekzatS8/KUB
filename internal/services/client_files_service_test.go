package services

import (
	"bytes"
	"context"
	"mime/multipart"
	"os"
	"path/filepath"
	"testing"
	"time"

	"turcompany/internal/models"
)

type fakeClientAccess struct {
	client *models.Client
	err    error
}

func (f *fakeClientAccess) GetByID(id int, userID, roleID int) (*models.Client, error) {
	return f.client, f.err
}

type fakeClientFileStore struct {
	upserted     *models.ClientFile
	upsertErr    error
	lastFilePath string
}

func (f *fakeClientFileStore) UpsertPrimary(ctx context.Context, clientID int64, category string, filePath string, mime *string, sizeBytes *int64, uploadedBy *int64) (*models.ClientFile, error) {
	f.lastFilePath = filePath
	if f.upsertErr != nil {
		return nil, f.upsertErr
	}
	m := mime
	s := sizeBytes
	u := uploadedBy
	return &models.ClientFile{
		ID:         1,
		ClientID:   clientID,
		Category:   category,
		FilePath:   filePath,
		Mime:       m,
		SizeBytes:  s,
		UploadedBy: u,
		CreatedAt:  time.Now(),
		IsPrimary:  true,
	}, nil
}

func (f *fakeClientFileStore) GetPrimaryByCategory(ctx context.Context, clientID int64, category string) (*models.ClientFile, error) {
	if f.upserted != nil {
		return f.upserted, nil
	}
	return nil, nil
}

func makeMultipartHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	r := multipart.NewReader(&body, w.Boundary())
	form, err := r.ReadForm(int64(body.Len()) + 1024)
	if err != nil {
		t.Fatalf("read form: %v", err)
	}
	files := form.File["file"]
	if len(files) == 0 {
		t.Fatal("file header missing")
	}
	return files[0]
}

func TestClientFilesServiceUploadPrimaryPhoto35x45(t *testing.T) {
	root := t.TempDir()
	store := &fakeClientFileStore{}
	svc := NewClientFilesService(root, &fakeClientAccess{client: &models.Client{ID: 7, OwnerID: 1}}, store)

	header := makeMultipartHeader(t, "photo.png", []byte("\x89PNG\r\n\x1a\nsmall"))
	rec, err := svc.UploadPrimary(context.Background(), 1, 1, 7, "photo35x45", header)
	if err != nil {
		t.Fatalf("upload primary: %v", err)
	}
	if rec.Category != "photo35x45" {
		t.Fatalf("unexpected category: %s", rec.Category)
	}
	if rec.FilePath == "" {
		t.Fatal("expected filepath")
	}
	abs := filepath.Join(root, filepath.FromSlash(rec.FilePath))
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("expected saved file at %s: %v", abs, err)
	}
}
