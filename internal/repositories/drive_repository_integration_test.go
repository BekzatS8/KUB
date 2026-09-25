package repositories

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"turcompany/internal/models"
)

// Интеграционный тест хранилища против настоящего PostgreSQL.
//
// Рекурсивные запросы (наследование доступа, путь, удаление поддерева) нельзя
// проверить заглушками — только на живой базе. По умолчанию тест пропускается;
// запуск:
//
//	DRIVE_TEST_DSN="postgres://user:pass@localhost:5432/db?sslmode=disable" go test ./internal/repositories -run DriveRepository
//
// База должна содержать таблицу users и миграцию 082_drive. Тест работает в
// транзакции-песочнице: всё созданное удаляется в конце.
func TestDriveRepositoryIntegration(t *testing.T) {
	dsn := os.Getenv("DRIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("DRIVE_TEST_DSN не задан — интеграционный тест хранилища пропущен")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	ctx := context.Background()

	// Изоляция от реальных данных: чистим только то, что создал тест.
	var userIDs []int
	cleanup := func() {
		_, _ = db.Exec(`DELETE FROM drive_nodes WHERE name LIKE 'drv-test-%'`)
		for _, id := range userIDs {
			_, _ = db.Exec(`DELETE FROM users WHERE id = $1`, id)
		}
	}
	defer cleanup()

	mkUser := func(email string, roleID int) int {
		var id int
		err := db.QueryRow(`
			INSERT INTO users (email, first_name, last_name, role_id, password_hash, is_active)
			VALUES ($1, 'Тест', $2, $3, 'x', TRUE) RETURNING id`, email, email, roleID).Scan(&id)
		if err != nil {
			t.Fatalf("create user %s: %v", email, err)
		}
		userIDs = append(userIDs, id)
		return id
	}
	admin := mkUser("drv-admin@test.local", 50)
	u1 := mkUser("drv-u1@test.local", 10)
	u2 := mkUser("drv-u2@test.local", 10)

	repo := NewDriveRepository(db)
	mk := func(parent *int64, kind, name, key string) *models.DriveNode {
		t.Helper()
		n := &models.DriveNode{ParentID: parent, Kind: kind, Name: name, StorageKey: key, CreatedBy: &admin}
		if kind == models.DriveKindFile {
			n.SizeBytes = 1000
			n.MimeType = "application/pdf"
		}
		if err := repo.CreateNode(ctx, n); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return n
	}

	// Дерево: drv-test-A/ drv-test-B/ drv-test-f1 ; drv-test-f2 в корне.
	folderA := mk(nil, models.DriveKindFolder, "drv-test-A", "")
	folderB := mk(&folderA.ID, models.DriveKindFolder, "drv-test-B", "")
	f1 := mk(&folderB.ID, models.DriveKindFile, "drv-test-f1.pdf", "drive/test/f1")
	f2 := mk(nil, models.DriveKindFile, "drv-test-f2.pdf", "drive/test/f2")

	t.Run("имена уникальны в папке без учёта регистра", func(t *testing.T) {
		dup := &models.DriveNode{Kind: models.DriveKindFolder, Name: "DRV-TEST-a"}
		if err := repo.CreateNode(ctx, dup); !errors.Is(err, ErrDriveNameTaken) {
			t.Fatalf("expected ErrDriveNameTaken, got %v", err)
		}
		// То же имя в другой папке — допустимо.
		other := &models.DriveNode{ParentID: &folderB.ID, Kind: models.DriveKindFolder, Name: "drv-test-A"}
		if err := repo.CreateNode(ctx, other); err != nil {
			t.Fatalf("same name in another folder must be allowed: %v", err)
		}
	})

	t.Run("папки выше файлов", func(t *testing.T) {
		items, err := repo.ListChildren(ctx, &folderB.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) < 2 || items[0].Kind != models.DriveKindFolder {
			t.Fatalf("expected folder first, got %+v", items)
		}
	})

	if err := repo.UpsertShares(ctx, folderA.ID, []int{u1}, nil, admin); err != nil {
		t.Fatalf("share A: %v", err)
	}
	// Файл внутри уже расшаренной папки выдан ещё и точечно.
	if err := repo.UpsertShares(ctx, f1.ID, []int{u1}, nil, admin); err != nil {
		t.Fatalf("share f1: %v", err)
	}

	t.Run("доступ к папке наследуется содержимым", func(t *testing.T) {
		cases := []struct {
			name   string
			node   int64
			user   int
			expect bool
		}{
			{"u1 → файл в глубине расшаренной папки", f1.ID, u1, true},
			{"u1 → вложенная папка", folderB.ID, u1, true},
			{"u1 → файл вне расшаренной папки", f2.ID, u1, false},
			{"u2 без доступов", f1.ID, u2, false},
		}
		for _, c := range cases {
			ok, err := repo.CanAccess(ctx, c.node, c.user)
			if err != nil {
				t.Fatal(err)
			}
			if ok != c.expect {
				t.Errorf("%s: want %v, got %v", c.name, c.expect, ok)
			}
		}
	})

	t.Run("«Доступные мне» без дублей", func(t *testing.T) {
		roots, err := repo.SharedRoots(ctx, u1)
		if err != nil {
			t.Fatal(err)
		}
		if len(roots) != 1 || roots[0].ID != folderA.ID {
			t.Fatalf("expected only folder A (f1 is covered by it), got %+v", roots)
		}
	})

	t.Run("путь от корня с отметкой общего звена", func(t *testing.T) {
		chain, err := repo.Ancestors(ctx, f1.ID, u1)
		if err != nil {
			t.Fatal(err)
		}
		if len(chain) != 3 || chain[0].ID != folderA.ID || chain[2].ID != f1.ID {
			t.Fatalf("unexpected chain %+v", chain)
		}
		if !chain[0].SharedWithUser || chain[1].SharedWithUser {
			t.Fatalf("shared flags wrong: %+v", chain)
		}
	})

	t.Run("истёкший доступ не действует", func(t *testing.T) {
		past := time.Now().Add(-time.Hour)
		if err := repo.UpsertShares(ctx, f2.ID, []int{u2}, &past, admin); err != nil {
			t.Fatal(err)
		}
		ok, err := repo.CanAccess(ctx, f2.ID, u2)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Error("expired share must not grant access")
		}
		roots, _ := repo.SharedRoots(ctx, u2)
		if len(roots) != 0 {
			t.Errorf("expired share must not be listed, got %+v", roots)
		}
		shares, err := repo.ListShares(ctx, f2.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(shares) != 1 || !shares[0].Expired {
			t.Fatalf("expected one expired share, got %+v", shares)
		}

		// Повторная выдача продлевает срок.
		future := time.Now().Add(24 * time.Hour)
		if err := repo.UpsertShares(ctx, f2.ID, []int{u2}, &future, admin); err != nil {
			t.Fatal(err)
		}
		if ok, _ := repo.CanAccess(ctx, f2.ID, u2); !ok {
			t.Error("renewed share must grant access")
		}
	})

	t.Run("удаление папки собирает ключи всего поддерева", func(t *testing.T) {
		objects, err := repo.DeleteNode(ctx, folderA.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(objects) != 1 || objects[0].StorageKey != "drive/test/f1" {
			t.Fatalf("expected f1 key collected, got %+v", objects)
		}
		if _, err := repo.GetNode(ctx, f1.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("nested file must be deleted by cascade, got %v", err)
		}
		shares, _ := repo.ListShares(ctx, folderA.ID)
		if len(shares) != 0 {
			t.Errorf("shares must be deleted by cascade, got %+v", shares)
		}
	})

	t.Run("роль администратора", func(t *testing.T) {
		if ok, _ := repo.UserHasRole(ctx, admin, 50); !ok {
			t.Error("admin must have role 50")
		}
		if ok, _ := repo.UserHasRole(ctx, u1, 50); ok {
			t.Error("u1 must not have role 50")
		}
	})
}
