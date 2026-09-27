package services

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/storage"
)

type recMessenger struct {
	texts []string
	files []string // имя файла → ссылка
	urls  []string
}

func (m *recMessenger) SendMessageText(_ context.Context, _ int, _, _, _, text string) error {
	m.texts = append(m.texts, text)
	return nil
}
func (m *recMessenger) SendFileURL(_ context.Context, _ int, chatID, transport, _, fileURL, fileName, _ string, _ int64) error {
	m.files = append(m.files, transport+":"+chatID+":"+fileName)
	m.urls = append(m.urls, fileURL)
	return nil
}

type recMailer struct {
	to       string
	contents map[string]string
}

func (m *recMailer) SendFiles(to, _, _ string, files []DriveMailFile) error {
	m.to = to
	m.contents = map[string]string{}
	for _, f := range files {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		m.contents[f.Name] = string(b)
	}
	return nil
}

// Переместить, копировать, свойства и отправить на настоящем PostgreSQL:
//
//	DRIVE_TEST_DSN="postgres://user:pass@localhost:5432/db?sslmode=disable" go test ./internal/services -run DriveOps
func TestDriveOpsIntegration(t *testing.T) {
	dsn := os.Getenv("DRIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("DRIVE_TEST_DSN не задан — интеграционный тест операций хранилища пропущен")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	cleanup := func() {
		_, _ = db.Exec(`DELETE FROM drive_nodes WHERE name LIKE 'dop-%'`)
		_, _ = db.Exec(`DELETE FROM users WHERE email LIKE 'dop-%@test.local'`)
	}
	cleanup()
	defer cleanup()

	var adminID, viewerID int
	for _, u := range []struct {
		email string
		role  int
		id    *int
	}{{"dop-admin@test.local", 50, &adminID}, {"dop-viewer@test.local", 10, &viewerID}} {
		if err := db.QueryRow(`INSERT INTO users (email, first_name, last_name, role_id, password_hash, is_active)
			VALUES ($1, 'Тест', 'Хранилище', $2, 'x', TRUE) RETURNING id`, u.email, u.role).Scan(u.id); err != nil {
			t.Fatal(err)
		}
	}
	admin := DriveActor{UserID: adminID, RoleID: 50}
	viewer := DriveActor{UserID: viewerID, RoleID: 10}

	store := storage.NewLocalStorage(t.TempDir())
	svc := NewDriveService(repositories.NewDriveRepository(db), store, DriveConfig{
		LinkSecret:   DriveLinkSecret([]byte("jwt-secret-for-tests-32-bytes-long!")),
		PublicAPIURL: "https://api.kub.test",
	})
	messenger, mailer := &recMessenger{}, &recMailer{}
	svc.SetMessenger(messenger)
	svc.SetMailer(mailer)

	folder := func(parent *int64, name string) *models.DriveNode {
		n, err := svc.CreateFolder(ctx, admin, parent, name)
		if err != nil {
			t.Fatalf("folder %s: %v", name, err)
		}
		return n
	}
	upload := func(parent *int64, name, content string) *models.DriveNode {
		n, err := svc.Upload(ctx, admin, parent, name, int64(len(content)), "", strings.NewReader(content))
		if err != nil {
			t.Fatalf("upload %s: %v", name, err)
		}
		return n
	}
	names := func(parent *int64) []string {
		items, err := svc.repo.ListChildren(ctx, parent)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, n := range items {
			out = append(out, n.Name)
		}
		return out
	}

	// dop-A/ { dop-inner/ { deep.txt }, a.pdf }, dop-B/, dop-x.txt в корне
	a := folder(nil, "dop-A")
	inner := folder(&a.ID, "dop-inner")
	deep := upload(&inner.ID, "deep.txt", "deep content")
	aFile := upload(&a.ID, "a.pdf", "%PDF a")
	b := folder(nil, "dop-B")
	x := upload(nil, "dop-x.txt", "x content")

	t.Run("копирование папки со всем содержимым и отдельными объектами", func(t *testing.T) {
		n, err := svc.Copy(ctx, admin, []int64{a.ID, x.ID}, &b.ID)
		if err != nil || n != 2 {
			t.Fatalf("copy: %d %v", n, err)
		}
		got := names(&b.ID)
		if len(got) != 2 || got[0] != "dop-A" || got[1] != "dop-x.txt" {
			t.Fatalf("unexpected copy content: %v", got)
		}
		items, _ := svc.repo.ListChildren(ctx, &b.ID)
		copyA := items[0]
		sub, _ := svc.repo.Subtree(ctx, copyA.ID)
		if len(sub) != 4 {
			t.Fatalf("copy must contain inner folder and both files, got %+v", sub)
		}
		for _, n := range sub {
			if n.Kind == models.DriveKindFile && (n.StorageKey == deep.StorageKey || n.StorageKey == aFile.StorageKey) {
				t.Fatal("copied file must get its own object")
			}
			if n.Name == "deep.txt" {
				rc, _, err := store.Open(ctx, n.StorageKey)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(rc)
				rc.Close()
				if string(body) != "deep content" {
					t.Fatalf("copied content mismatch: %q", body)
				}
			}
		}
		// Повторное копирование — номер у имени.
		if _, err := svc.Copy(ctx, admin, []int64{x.ID}, &b.ID); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(names(&b.ID), "|"); got != "dop-A|dop-x (2).txt|dop-x.txt" {
			t.Fatalf("second copy must be numbered, got %v", got)
		}
	})

	t.Run("папку нельзя положить в неё саму или в подпапку", func(t *testing.T) {
		if _, err := svc.Move(ctx, admin, []int64{a.ID}, &inner.ID); !errors.Is(err, ErrDriveMoveIntoSelf) {
			t.Fatalf("move into own subfolder must fail, got %v", err)
		}
		if _, err := svc.Copy(ctx, admin, []int64{a.ID}, &a.ID); !errors.Is(err, ErrDriveMoveIntoSelf) {
			t.Fatalf("copy into itself must fail, got %v", err)
		}
	})

	t.Run("перемещение с номером при совпадении имени", func(t *testing.T) {
		n, err := svc.Move(ctx, admin, []int64{x.ID}, &b.ID)
		if err != nil || n != 1 {
			t.Fatalf("move: %d %v", n, err)
		}
		moved, _ := svc.repo.GetNode(ctx, x.ID)
		if moved.ParentID == nil || *moved.ParentID != b.ID || moved.Name != "dop-x (3).txt" {
			t.Fatalf("moved node mismatch: %+v", moved)
		}
		// Уже в этой папке — ничего не делаем.
		if n, _ := svc.Move(ctx, admin, []int64{x.ID}, &b.ID); n != 0 {
			t.Fatalf("move into the same folder must be a no-op, got %d", n)
		}
	})

	t.Run("только администратор", func(t *testing.T) {
		if _, err := svc.Move(ctx, viewer, []int64{x.ID}, nil); !errors.Is(err, ErrDriveForbidden) {
			t.Fatalf("viewer must not move, got %v", err)
		}
		if _, err := svc.Copy(ctx, viewer, []int64{x.ID}, nil); !errors.Is(err, ErrDriveForbidden) {
			t.Fatalf("viewer must not copy, got %v", err)
		}
	})

	t.Run("свойства папки и файла", func(t *testing.T) {
		props, err := svc.Properties(ctx, admin, a.ID)
		if err != nil {
			t.Fatal(err)
		}
		if *props.Files != 2 || *props.Folders != 1 || *props.TotalBytes != int64(len("deep content")+len("%PDF a")) {
			t.Fatalf("folder stats mismatch: files=%d folders=%d bytes=%d", *props.Files, *props.Folders, *props.TotalBytes)
		}
		fp, err := svc.Properties(ctx, admin, deep.ID)
		if err != nil || len(fp.Path) != 2 || fp.Path[0].ID != a.ID || fp.Files != nil {
			t.Fatalf("file properties mismatch: %+v %v", fp, err)
		}
		if _, err := svc.Properties(ctx, viewer, deep.ID); !errors.Is(err, ErrDriveNotFound) {
			t.Fatalf("viewer without access must not see properties, got %v", err)
		}
	})

	t.Run("отправка в WhatsApp: текст и ссылки на файлы", func(t *testing.T) {
		res, err := svc.Send(ctx, admin, DriveSendRequest{
			IDs: []int64{aFile.ID, deep.ID}, Channel: "whatsapp", To: "+7 (700) 111-22-33", Text: "Документы",
		})
		if err != nil || res.Sent != 2 {
			t.Fatalf("send: %+v %v", res, err)
		}
		if len(messenger.texts) != 1 || messenger.files[0] != "whatsapp:77001112233:a.pdf" {
			t.Fatalf("unexpected messenger calls: %v %v", messenger.texts, messenger.files)
		}
		if !strings.HasPrefix(messenger.urls[0], "https://api.kub.test/api/v1/drive/raw/") {
			t.Fatalf("file link must be absolute public url, got %s", messenger.urls[0])
		}
		// Ссылка действительно открывает файл.
		token := strings.TrimPrefix(messenger.urls[0], "https://api.kub.test/api/v1/drive/raw/")
		content, err := svc.Open(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(content.Reader)
		content.Reader.Close()
		if !bytes.Equal(body, []byte("%PDF a")) {
			t.Fatalf("link content mismatch: %q", body)
		}
	})

	t.Run("папку отправить нельзя, чужой файл — тоже", func(t *testing.T) {
		if _, err := svc.Send(ctx, admin, DriveSendRequest{IDs: []int64{a.ID}, Channel: "telegram", To: "user"}); !errors.Is(err, ErrDriveSendFolder) {
			t.Fatalf("folder send must fail, got %v", err)
		}
		if _, err := svc.Send(ctx, viewer, DriveSendRequest{IDs: []int64{aFile.ID}, Channel: "email", To: "a@b.kz"}); !errors.Is(err, ErrDriveNotFound) {
			t.Fatalf("viewer without access must not send, got %v", err)
		}
	})

	t.Run("отправка на почту с вложениями", func(t *testing.T) {
		if _, err := svc.Send(ctx, admin, DriveSendRequest{IDs: []int64{aFile.ID}, Channel: "email", To: "not-an-email"}); !errors.Is(err, ErrDriveSendEmail) {
			t.Fatalf("bad email must be rejected, got %v", err)
		}
		res, err := svc.Send(ctx, admin, DriveSendRequest{IDs: []int64{aFile.ID, deep.ID}, Channel: "email", To: "client@example.kz"})
		if err != nil || res.Sent != 2 {
			t.Fatalf("email send: %+v %v", res, err)
		}
		if mailer.to != "client@example.kz" || mailer.contents["deep.txt"] != "deep content" || mailer.contents["a.pdf"] != "%PDF a" {
			t.Fatalf("email attachments mismatch: %+v", mailer)
		}
	})
}
