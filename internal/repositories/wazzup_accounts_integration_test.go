package repositories

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// Подключения Wazzup по аккаунтам на настоящем PostgreSQL: ON CONFLICT по
// частичному уникальному индексу и закрепление старого подключения нельзя
// проверить заглушками.
//
//	WAZZUP_TEST_DSN="postgres://user:pass@localhost:5432/db?sslmode=disable" go test ./internal/repositories -run WazzupAccounts
//
// Нужна база с миграциями 013 и 083. Всё созданное тестом удаляется.
func TestWazzupAccountsIntegration(t *testing.T) {
	dsn := os.Getenv("WAZZUP_TEST_DSN")
	if dsn == "" {
		t.Skip("WAZZUP_TEST_DSN не задан — интеграционный тест аккаунтов Wazzup пропущен")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	// Остатки прошлого прогона, если он упал до уборки.
	_, _ = db.Exec(`DELETE FROM wazzup_integrations WHERE owner_user_id IN (SELECT id FROM users WHERE email LIKE 'wz-%@test.local')`)
	_, _ = db.Exec(`DELETE FROM users WHERE email LIKE 'wz-%@test.local'`)

	var owner, otherAdmin int
	for _, email := range []string{"wz-owner@test.local", "wz-other@test.local"} {
		var id int
		if err := db.QueryRow(`INSERT INTO users (email, first_name, last_name, role_id, password_hash, is_active)
			VALUES ($1, 'Тест', 'Wazzup', 50, 'x', TRUE) RETURNING id`, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if owner == 0 {
			owner = id
		} else {
			otherAdmin = id
		}
	}
	defer func() {
		_, _ = db.Exec(`DELETE FROM wazzup_integrations WHERE owner_user_id IN ($1, $2)`, owner, otherAdmin)
		_, _ = db.Exec(`DELETE FROM users WHERE id IN ($1, $2)`, owner, otherAdmin)
	}()

	repo := NewWazzupRepository(db)

	// Два подключения «до обновления» (account IS NULL); второе — свежее.
	// Именно его система реально использовала: фоллбэк брал последнее.
	var oldID, liveID int
	if err := db.QueryRow(`INSERT INTO wazzup_integrations (owner_user_id, api_key_enc, crm_key_hash, webhook_token, enabled, updated_at)
		VALUES ($1, '', 'h-old', 'tok-old', TRUE, NOW() - interval '30 days') RETURNING id`, otherAdmin).Scan(&oldID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO wazzup_integrations (owner_user_id, api_key_enc, crm_key_hash, webhook_token, enabled, updated_at)
		VALUES ($1, '', 'h-live', 'tok-live', TRUE, NOW()) RETURNING id`, owner).Scan(&liveID); err != nil {
		t.Fatal(err)
	}

	t.Run("закрепляется реально работавшее подключение", func(t *testing.T) {
		id, err := repo.AdoptLegacyIntegration(ctx, "child")
		if err != nil {
			t.Fatal(err)
		}
		if id != liveID {
			t.Fatalf("must adopt the most recently used connection %d, got %d", liveID, id)
		}
		got, err := repo.GetIntegrationByAccount(ctx, "child")
		if err != nil || got == nil || got.ID != liveID || got.WebhookToken != "tok-live" || got.Account != "child" {
			t.Fatalf("adopted connection must keep its token: %+v %v", got, err)
		}
		// Повторный старт ничего не меняет.
		if again, err := repo.AdoptLegacyIntegration(ctx, "child"); err != nil || again != 0 {
			t.Fatalf("second adoption must be a no-op, got %d %v", again, err)
		}
	})

	t.Run("подключение второго аккаунта — новая запись со своим токеном", func(t *testing.T) {
		id, token, err := repo.UpsertIntegrationByAccount(ctx, "main", owner, "main-key", "h1", "", true)
		if err != nil {
			t.Fatal(err)
		}
		if id == liveID || token == "tok-live" || token == "" {
			t.Fatalf("main must get its own connection and token, got id=%d token=%q", id, token)
		}
		// Повторное подключение: токен и владелец сохраняются, ключ и URL меняются.
		id2, token2, err := repo.UpsertIntegrationByAccount(ctx, "main", otherAdmin, "main-key", "h2", "https://api/x/"+token, true)
		if err != nil {
			t.Fatal(err)
		}
		if id2 != id || token2 != token {
			t.Fatalf("reconnect must keep id and token: %d/%q vs %d/%q", id, token, id2, token2)
		}
		got, _ := repo.GetIntegrationByAccount(ctx, "main")
		if got.CRMKeyHash != "h2" || got.WebhooksURI != "https://api/x/"+token {
			t.Fatalf("reconnect must update key and uri: %+v", got)
		}
		if got.OwnerUserID != owner {
			t.Fatalf("owner must not change on reconnect (branch fallback depends on it), got %d", got.OwnerUserID)
		}
		// Повтор с пустым URL не затирает сохранённый.
		if _, _, err := repo.UpsertIntegrationByAccount(ctx, "main", owner, "main-key", "h3", "", true); err != nil {
			t.Fatal(err)
		}
		got, _ = repo.GetIntegrationByAccount(ctx, "main")
		if got.WebhooksURI != "https://api/x/"+token {
			t.Fatalf("empty uri must not wipe stored one: %q", got.WebhooksURI)
		}
	})

	t.Run("один админ может подключить оба аккаунта", func(t *testing.T) {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM wazzup_integrations WHERE owner_user_id = $1 AND account IS NOT NULL`, owner).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 2 {
			t.Fatalf("owner must hold both accounts, got %d", n)
		}
	})

	t.Run("по токену вебхука находится подключение с аккаунтом", func(t *testing.T) {
		got, err := repo.GetIntegrationByToken(ctx, "tok-live")
		if err != nil || got == nil || got.Account != "child" {
			t.Fatalf("token lookup must return account: %+v %v", got, err)
		}
	})
}
