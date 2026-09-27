package repositories

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"turcompany/internal/models"
)

// Доступ к чатам номера на настоящем PostgreSQL (миграция 085):
//
//	WAZZUP_TEST_DSN="postgres://user:pass@localhost:5432/db?sslmode=disable" go test ./internal/repositories -run WazzupChannelRoles
//
// Нужна база с таблицами users, branches, departments, wazzup_integrations,
// wazzup_channels и миграцией 085. Всё созданное тестом удаляется.
func TestWazzupChannelRolesIntegration(t *testing.T) {
	dsn := os.Getenv("WAZZUP_TEST_DSN")
	if dsn == "" {
		t.Skip("WAZZUP_TEST_DSN не задан — интеграционный тест доступа к номерам пропущен")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	_, _ = db.Exec(`DELETE FROM wazzup_integrations WHERE webhook_token = 'wzr-token'`)
	_, _ = db.Exec(`DELETE FROM users WHERE email LIKE 'wzr-%@test.local'`)

	var active, blocked int
	if err := db.QueryRow(`INSERT INTO users (email, first_name, last_name, role_id, password_hash, is_active)
		VALUES ('wzr-a@test.local', 'Кундыз', 'Алсейтова', 10, 'x', TRUE) RETURNING id`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO users (email, first_name, last_name, role_id, password_hash, is_active)
		VALUES ('wzr-b@test.local', 'Уволен', 'Сотрудник', 10, 'x', FALSE) RETURNING id`).Scan(&blocked); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = db.Exec(`DELETE FROM users WHERE id IN ($1, $2)`, active, blocked) }()

	var integrationID int
	if err := db.QueryRow(`INSERT INTO wazzup_integrations (owner_user_id, api_key_enc, crm_key_hash, webhook_token, enabled)
		VALUES ($1, '', '', 'wzr-token', TRUE) RETURNING id`, active).Scan(&integrationID); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = db.Exec(`DELETE FROM wazzup_integrations WHERE id = $1`, integrationID) }()

	repo := NewWazzupRepository(db)
	if err := repo.UpsertChannels(ctx, integrationID, []models.WazzupChannel{{ExternalChannelID: "wzr-ch", Transport: "whatsapp", Name: "Алматы"}}); err != nil {
		t.Fatal(err)
	}
	channels, err := repo.ListChannels(ctx, integrationID)
	if err != nil || len(channels) != 1 {
		t.Fatalf("list channels: %v %+v", err, channels)
	}
	ch := channels[0]
	if ch.RolesConfigured {
		t.Fatal("new channel must start with automatic access")
	}

	if err := repo.ReplaceChannelRoles(ctx, ch.ID, []ChannelUserRole{{UserID: active, Role: "seller", AllowGetNewClients: true}}); err != nil {
		t.Fatal(err)
	}
	channels, _ = repo.ListChannels(ctx, integrationID)
	if !channels[0].RolesConfigured {
		t.Fatal("channel must be marked as manually configured")
	}
	roles, err := repo.ListChannelRoles(ctx, ch.ID)
	if err != nil || len(roles) != 1 || roles[0].UserID != active || roles[0].Role != "seller" || !roles[0].AllowGetNewClients {
		t.Fatalf("saved roles mismatch: %v %+v", err, roles)
	}

	// Повторная синхронизация канала не сбрасывает ручную настройку.
	if err := repo.UpsertChannels(ctx, integrationID, []models.WazzupChannel{{ExternalChannelID: "wzr-ch", Transport: "whatsapp", Name: "Алматы 2"}}); err != nil {
		t.Fatal(err)
	}
	channels, _ = repo.ListChannels(ctx, integrationID)
	if !channels[0].RolesConfigured {
		t.Fatal("channel sync must keep manual access")
	}

	// Недопустимая роль отклоняется базой.
	if err := repo.ReplaceChannelRoles(ctx, ch.ID, []ChannelUserRole{{UserID: active, Role: "owner"}}); err == nil {
		t.Fatal("unknown role must be rejected by the check constraint")
	}
	if roles, _ := repo.ListChannelRoles(ctx, ch.ID); len(roles) != 1 {
		t.Fatalf("failed replace must roll back, got %+v", roles)
	}

	candidates, err := repo.ListRoleCandidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var sawActive, sawBlocked bool
	for _, c := range candidates {
		if c.ID == active {
			sawActive = c.Name == "Алсейтова Кундыз"
		}
		if c.ID == blocked {
			sawBlocked = true
		}
	}
	if !sawActive || sawBlocked {
		t.Fatalf("candidates must list active employees with full name only: active=%v blocked=%v", sawActive, sawBlocked)
	}

	if err := repo.ResetChannelRoles(ctx, ch.ID); err != nil {
		t.Fatal(err)
	}
	channels, _ = repo.ListChannels(ctx, integrationID)
	roles, _ = repo.ListChannelRoles(ctx, ch.ID)
	if channels[0].RolesConfigured || len(roles) != 0 {
		t.Fatalf("reset must return automatic access: configured=%v roles=%+v", channels[0].RolesConfigured, roles)
	}
	if err := repo.ResetChannelRoles(ctx, 999999999); err != sql.ErrNoRows {
		t.Fatalf("missing channel must report ErrNoRows, got %v", err)
	}
}
