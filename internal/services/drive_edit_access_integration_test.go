package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/storage"
)

type recNotifier struct {
	events []models.FeedEvent
}

func (n *recNotifier) Create(_ context.Context, requesterID int, eventType string, payload json.RawMessage, _ *int) (*models.FeedEvent, error) {
	e := models.FeedEvent{RequesterID: requesterID, EventType: eventType, Payload: payload}
	n.events = append(n.events, e)
	return &e, nil
}

// Доступ «редактирование» к папке на настоящем PostgreSQL:
//
//	DRIVE_TEST_DSN="postgres://user:pass@localhost:5432/db?sslmode=disable" go test ./internal/services -run DriveEditAccess
func TestDriveEditAccessIntegration(t *testing.T) {
	dsn := os.Getenv("DRIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("DRIVE_TEST_DSN не задан — интеграционный тест доступа на редактирование пропущен")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	cleanup := func() {
		_, _ = db.Exec(`DELETE FROM drive_nodes WHERE name LIKE 'eda-%'`)
		_, _ = db.Exec(`DELETE FROM users WHERE email LIKE 'eda-%@test.local'`)
	}
	cleanup()
	defer cleanup()

	mkUser := func(email string, role int) int {
		var id int
		if err := db.QueryRow(`INSERT INTO users (email, first_name, last_name, role_id, password_hash, is_active)
			VALUES ($1, 'Тест', 'Менеджер', $2, 'x', TRUE) RETURNING id`, email, role).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	admin := DriveActor{UserID: mkUser("eda-admin@test.local", 50), RoleID: 50}
	manager := DriveActor{UserID: mkUser("eda-manager@test.local", 10), RoleID: 10}
	reader := DriveActor{UserID: mkUser("eda-reader@test.local", 10), RoleID: 10}

	notifier := &recNotifier{}
	svc := NewDriveService(repositories.NewDriveRepository(db), storage.NewLocalStorage(t.TempDir()),
		DriveConfig{LinkSecret: DriveLinkSecret([]byte("jwt-secret-for-tests-32-bytes-long!"))})
	svc.SetNotifier(notifier)

	branch, err := svc.CreateFolder(ctx, admin, nil, "eda-Алматы")
	if err != nil {
		t.Fatal(err)
	}
	own, err := svc.CreateFolder(ctx, admin, &branch.ID, "eda-Менеджер")
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.CreateFolder(ctx, admin, &branch.ID, "eda-Другой")
	if err != nil {
		t.Fatal(err)
	}
	// Менеджеру — редактирование своей папки, второму — только просмотр.
	if _, err := svc.Share(ctx, admin, own.ID, DriveShareTargets{UserIDs: []int{manager.UserID}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, admin, own.ID, DriveShareTargets{UserIDs: []int{reader.UserID}, Access: models.DriveAccessView}, nil); err != nil {
		t.Fatal(err)
	}

	var sub *models.DriveNode
	var file *models.DriveNode

	t.Run("внутри своей папки можно всё", func(t *testing.T) {
		l, err := svc.List(ctx, manager, &own.ID)
		if err != nil || !l.CanEdit || l.CanManage {
			t.Fatalf("manager must be able to edit inside the shared folder: %+v %v", l, err)
		}
		if sub, err = svc.CreateFolder(ctx, manager, &own.ID, "Клиенты"); err != nil {
			t.Fatal(err)
		}
		if file, err = svc.Upload(ctx, manager, &own.ID, "договор.pdf", 5, "", strings.NewReader("hello")); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Rename(ctx, manager, file.ID, "договор-1.pdf"); err != nil {
			t.Fatal(err)
		}
		if n, err := svc.Copy(ctx, manager, []int64{file.ID}, &sub.ID); err != nil || n != 1 {
			t.Fatalf("copy inside own folder: %d %v", n, err)
		}
		if n, err := svc.Move(ctx, manager, []int64{file.ID}, &sub.ID); err != nil || n != 1 {
			t.Fatalf("move inside own folder: %d %v", n, err)
		}
	})

	t.Run("наружу нельзя", func(t *testing.T) {
		if _, err := svc.Rename(ctx, manager, own.ID, "eda-чужое"); !errors.Is(err, ErrDriveForbidden) {
			t.Fatalf("shared folder itself must not be renamed, got %v", err)
		}
		if _, err := svc.Delete(ctx, manager, own.ID); !errors.Is(err, ErrDriveForbidden) {
			t.Fatalf("shared folder itself must not be deleted, got %v", err)
		}
		if _, err := svc.CreateFolder(ctx, manager, &branch.ID, "eda-x"); !errors.Is(err, ErrDriveNotFound) && !errors.Is(err, ErrDriveForbidden) {
			t.Fatalf("must not create above the shared folder, got %v", err)
		}
		if _, err := svc.CreateFolder(ctx, manager, nil, "eda-x"); !errors.Is(err, ErrDriveForbidden) {
			t.Fatalf("must not create in storage root, got %v", err)
		}
		if _, err := svc.Move(ctx, manager, []int64{sub.ID}, &other.ID); err == nil {
			t.Fatal("must not move into a folder without edit access")
		}
		if _, err := svc.Copy(ctx, manager, []int64{sub.ID}, &branch.ID); err == nil {
			t.Fatal("must not copy above the shared folder")
		}
		if _, err := svc.ListTrash(ctx, manager); !errors.Is(err, ErrDriveForbidden) {
			t.Fatalf("trash is admin only, got %v", err)
		}
	})

	t.Run("просмотр — ничего не менять", func(t *testing.T) {
		l, err := svc.List(ctx, reader, &own.ID)
		if err != nil || l.CanEdit {
			t.Fatalf("view access must not edit: %+v %v", l, err)
		}
		if _, err := svc.CreateFolder(ctx, reader, &own.ID, "x"); !errors.Is(err, ErrDriveForbidden) {
			t.Fatalf("view access must not create, got %v", err)
		}
		if _, err := svc.Delete(ctx, reader, sub.ID); !errors.Is(err, ErrDriveForbidden) {
			t.Fatalf("view access must not delete, got %v", err)
		}
	})

	t.Run("удаление менеджера — в корзину и в Ленту, отклонение восстанавливает", func(t *testing.T) {
		n, err := svc.DeleteMany(ctx, manager, []int64{sub.ID})
		if err != nil || n != 1 {
			t.Fatalf("delete: %d %v", n, err)
		}
		if len(notifier.events) != 1 || notifier.events[0].EventType != models.FeedEventTypeDriveDelete || notifier.events[0].RequesterID != manager.UserID {
			t.Fatalf("feed event expected, got %+v", notifier.events)
		}
		var p driveDeletePayload
		_ = json.Unmarshal(notifier.events[0].Payload, &p)
		if p.Count != 1 || p.Items[0].Name != "Клиенты" || !strings.Contains(p.Folder, "eda-Менеджер") {
			t.Fatalf("feed payload mismatch: %+v", p)
		}
		trash, _ := svc.ListTrash(ctx, admin)
		inTrash := false
		for _, it := range trash.Items {
			inTrash = inTrash || it.ID == sub.ID
		}
		if !inTrash {
			t.Fatal("manager's delete must land in trash")
		}
		if err := svc.RestoreFromFeed(ctx, admin.UserID, notifier.events[0].Payload); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Properties(ctx, manager, sub.ID); err != nil {
			t.Fatalf("rejected delete must restore the folder, got %v", err)
		}
		// Удаление администратором в Ленту не пишется.
		if _, err := svc.Delete(ctx, admin, sub.ID); err != nil {
			t.Fatal(err)
		}
		if len(notifier.events) != 1 {
			t.Fatalf("admin delete must not create feed events, got %d", len(notifier.events))
		}
	})
}
