package services

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/storage"
)

// Корзина хранилища на настоящем PostgreSQL:
//
//	DRIVE_TEST_DSN="postgres://user:pass@localhost:5432/db?sslmode=disable" go test ./internal/services -run DriveTrash
func TestDriveTrashIntegration(t *testing.T) {
	dsn := os.Getenv("DRIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("DRIVE_TEST_DSN не задан — интеграционный тест корзины пропущен")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	cleanup := func() {
		_, _ = db.Exec(`DELETE FROM drive_nodes WHERE name LIKE 'trs-%'`)
		_, _ = db.Exec(`DELETE FROM users WHERE email LIKE 'trs-%@test.local'`)
	}
	cleanup()
	defer cleanup()

	var adminID, viewerID int
	if err := db.QueryRow(`INSERT INTO users (email, first_name, last_name, role_id, password_hash, is_active)
		VALUES ('trs-admin@test.local', 'Тест', 'Корзина', 50, 'x', TRUE) RETURNING id`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO users (email, first_name, last_name, role_id, password_hash, is_active)
		VALUES ('trs-viewer@test.local', 'Тест', 'Корзина', 10, 'x', TRUE) RETURNING id`).Scan(&viewerID); err != nil {
		t.Fatal(err)
	}
	admin := DriveActor{UserID: adminID, RoleID: 50}
	viewer := DriveActor{UserID: viewerID, RoleID: 10}

	store := storage.NewLocalStorage(t.TempDir())
	repo := repositories.NewDriveRepository(db)
	svc := NewDriveService(repo, store, DriveConfig{LinkSecret: DriveLinkSecret([]byte("jwt-secret-for-tests-32-bytes-long!"))})

	folder := func(parent *int64, name string) *models.DriveNode {
		n, err := svc.CreateFolder(ctx, admin, parent, name)
		if err != nil {
			t.Fatalf("folder %s: %v", name, err)
		}
		return n
	}
	upload := func(parent *int64, name string) *models.DriveNode {
		n, err := svc.Upload(ctx, admin, parent, name, 5, "", strings.NewReader("hello"))
		if err != nil {
			t.Fatalf("upload %s: %v", name, err)
		}
		return n
	}
	rootNames := func() string {
		l, err := svc.List(ctx, admin, nil)
		if err != nil {
			t.Fatal(err)
		}
		names := []string{}
		for _, n := range l.Items {
			if strings.HasPrefix(n.Name, "trs-") {
				names = append(names, n.Name)
			}
		}
		return strings.Join(names, "|")
	}
	objectExists := func(key string) bool {
		rc, _, err := store.Open(ctx, key)
		if err != nil {
			return false
		}
		rc.Close()
		return true
	}

	// trs-A/ { trs-B/ { g.pdf }, f.pdf }
	a := folder(nil, "trs-A")
	b := folder(&a.ID, "trs-B")
	g := upload(&b.ID, "g.pdf")
	f := upload(&a.ID, "f.pdf")
	if _, err := svc.Share(ctx, admin, a.ID, DriveShareTargets{UserIDs: []int{viewerID}}, nil); err != nil {
		t.Fatal(err)
	}

	t.Run("удаление переносит в корзину и всё прячет", func(t *testing.T) {
		if _, err := svc.Delete(ctx, admin, g.ID); err != nil {
			t.Fatal(err)
		}
		n, err := svc.Delete(ctx, admin, a.ID)
		if err != nil || n != 1 {
			t.Fatalf("trash A must create one trash entry, got %d %v", n, err)
		}
		if rootNames() != "" {
			t.Fatalf("trashed folder must disappear from the list, got %q", rootNames())
		}
		if _, err := svc.Link(ctx, admin, f.ID, "", false); !errors.Is(err, ErrDriveNotFound) {
			t.Fatalf("trashed file must not open, got %v", err)
		}
		if ok, _ := repo.CanAccess(ctx, f.ID, viewerID); ok {
			t.Fatal("shared access must not work for trashed content")
		}
		roots, _ := repo.SharedRoots(ctx, viewerID)
		if len(roots) != 0 {
			t.Fatalf("trashed shared folder must disappear for the viewer, got %+v", roots)
		}
		if !objectExists(f.StorageKey) {
			t.Fatal("trash must keep the object in storage")
		}
		trash, err := svc.ListTrash(ctx, admin)
		if err != nil {
			t.Fatal(err)
		}
		byName := map[string]repositories.DriveTrashItem{}
		for _, it := range trash.Items {
			byName[it.Name] = it
		}
		if len(byName) != 2 || byName["trs-A"].Files != 1 || byName["g.pdf"].ParentName != "trs-B" || !byName["g.pdf"].ParentTrashed {
			t.Fatalf("unexpected trash: %+v", trash.Items)
		}
		if _, err := svc.ListTrash(ctx, viewer); !errors.Is(err, ErrDriveForbidden) {
			t.Fatalf("only admin sees trash, got %v", err)
		}
	})

	t.Run("имя удалённого можно занять", func(t *testing.T) {
		folder(nil, "trs-A")
	})

	t.Run("восстановление с номером и доступами", func(t *testing.T) {
		if n, err := svc.Restore(ctx, admin, []int64{a.ID}); err != nil || n != 1 {
			t.Fatalf("restore: %d %v", n, err)
		}
		if got := rootNames(); got != "trs-A|trs-A (2)" {
			t.Fatalf("restored folder must get a number, got %q", got)
		}
		if ok, _ := repo.CanAccess(ctx, f.ID, viewerID); !ok {
			t.Fatal("restored content must regain its shares")
		}
		// g удаляли отдельно — он остаётся в корзине и возвращается в живую
		// теперь папку B.
		if _, err := svc.Link(ctx, admin, g.ID, "", false); !errors.Is(err, ErrDriveNotFound) {
			t.Fatal("separately trashed file must stay in trash")
		}
		if _, err := svc.Restore(ctx, admin, []int64{g.ID}); err != nil {
			t.Fatal(err)
		}
		back, err := repo.GetNode(ctx, g.ID)
		if err != nil || back.ParentID == nil || *back.ParentID != b.ID {
			t.Fatalf("file must return to its folder, got %+v %v", back, err)
		}
	})

	t.Run("восстановление в корень, если папки больше нет", func(t *testing.T) {
		if _, err := svc.Delete(ctx, admin, g.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Delete(ctx, admin, b.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Restore(ctx, admin, []int64{g.ID}); err != nil {
			t.Fatal(err)
		}
		back, _ := repo.GetNode(ctx, g.ID)
		if back == nil || back.ParentID != nil {
			t.Fatalf("file of a trashed folder must be restored to root, got %+v", back)
		}
		_, _ = db.Exec(`UPDATE drive_nodes SET name = 'trs-g.pdf' WHERE id = $1`, g.ID)
	})

	t.Run("удалить навсегда", func(t *testing.T) {
		if _, err := svc.Purge(ctx, admin, []int64{f.ID}); !errors.Is(err, ErrDriveNotFound) {
			t.Fatalf("live file must not be purged, got %v", err)
		}
		if _, err := svc.Delete(ctx, admin, f.ID); err != nil {
			t.Fatal(err)
		}
		n, err := svc.Purge(ctx, admin, []int64{f.ID})
		if err != nil || n != 1 {
			t.Fatalf("purge: %d %v", n, err)
		}
		if objectExists(f.StorageKey) {
			t.Fatal("purge must delete the object from storage")
		}
		if _, err := svc.Restore(ctx, admin, []int64{f.ID}); !errors.Is(err, ErrDriveNotFound) {
			t.Fatalf("purged file must not be restorable, got %v", err)
		}
	})

	t.Run("очистить корзину", func(t *testing.T) {
		if _, err := svc.Delete(ctx, admin, g.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.EmptyTrash(ctx, admin); err != nil {
			t.Fatal(err)
		}
		trash, _ := svc.ListTrash(ctx, admin)
		for _, it := range trash.Items {
			if strings.HasPrefix(it.Name, "trs-") || it.Name == "g.pdf" {
				t.Fatalf("trash must be empty, got %+v", trash.Items)
			}
		}
		if objectExists(g.StorageKey) {
			t.Fatal("emptying trash must delete objects")
		}
	})
}
