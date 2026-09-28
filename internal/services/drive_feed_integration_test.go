package services

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/storage"
)

// Удаление сотрудником в хранилище → событие в Ленте; «Отклонить» в Ленте
// возвращает удалённое. Настоящий PostgreSQL:
//
//	DRIVE_TEST_DSN=... go test ./internal/services -run DriveFeed
func TestDriveFeedIntegration(t *testing.T) {
	dsn := os.Getenv("DRIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("DRIVE_TEST_DSN не задан — интеграционный тест Ленты хранилища пропущен")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	var adminID, managerID int
	cleanup := func() {
		_, _ = db.Exec(`DELETE FROM feed_events WHERE event_type = 'drive_delete' AND requester_id IN (SELECT id FROM users WHERE email LIKE 'dfe-%@test.local')`)
		_, _ = db.Exec(`DELETE FROM drive_nodes WHERE name LIKE 'dfe-%'`)
		_, _ = db.Exec(`DELETE FROM users WHERE email LIKE 'dfe-%@test.local'`)
	}
	cleanup()
	defer cleanup()
	for _, u := range []struct {
		email string
		role  int
		id    *int
	}{{"dfe-admin@test.local", 50, &adminID}, {"dfe-manager@test.local", 10, &managerID}} {
		if err := db.QueryRow(`INSERT INTO users (email, first_name, last_name, role_id, password_hash, is_active)
			VALUES ($1, 'Айгерим', 'Менеджер', $2, 'x', TRUE) RETURNING id`, u.email, u.role).Scan(u.id); err != nil {
			t.Fatal(err)
		}
	}
	admin := DriveActor{UserID: adminID, RoleID: 50}
	manager := DriveActor{UserID: managerID, RoleID: 10}

	drive := NewDriveService(repositories.NewDriveRepository(db), storage.NewLocalStorage(t.TempDir()),
		DriveConfig{LinkSecret: DriveLinkSecret([]byte("jwt-secret-for-tests-32-bytes-long!"))})
	feed := NewFeedEventService(repositories.NewFeedEventRepository(db), repositories.NewUserRepository(db), nil, nil, nil, nil, nil)
	drive.SetNotifier(feed)
	feed.SetDriveRestorer(drive)

	own, err := drive.CreateFolder(ctx, admin, nil, "dfe-Менеджер")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := drive.Share(ctx, admin, own.ID, DriveShareTargets{UserIDs: []int{managerID}}, nil); err != nil {
		t.Fatal(err)
	}
	f, err := drive.Upload(ctx, manager, &own.ID, "скан.pdf", 5, "", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := drive.DeleteMany(ctx, manager, []int64{f.ID}); err != nil {
		t.Fatal(err)
	}

	events, err := feed.List(ctx, adminID, 50, models.FeedEventStatusPending, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	var ev *models.FeedEvent
	for _, e := range events {
		if e.EventType == models.FeedEventTypeDriveDelete && e.RequesterID == managerID {
			ev = e
		}
	}
	if ev == nil || !strings.Contains(string(ev.Payload), "скан.pdf") || ev.RequesterName == "" {
		t.Fatalf("admin must see the manager's delete in the feed, got %+v", ev)
	}
	if _, err := drive.Properties(ctx, manager, f.ID); err == nil {
		t.Fatal("deleted file must be hidden until restored")
	}
	if _, err := feed.Reject(ctx, ev.ID, adminID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := drive.Properties(ctx, manager, f.ID); err != nil {
		t.Fatalf("reject in the feed must restore the file, got %v", err)
	}
}
