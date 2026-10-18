package repositories

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// Ответственный за лиды и сделки на настоящем PostgreSQL: снятие при
// увольнении (RETURNING воронок), атомарный «Взять себе» и чтение сделки без
// ответственного (owner_id IS NULL после миграции 093).
//
//	OWNER_TEST_DSN="postgres://user:pass@localhost:5432/db?sslmode=disable" go test ./internal/repositories -run OwnerRelease
//
// Нужна база со всеми миграциями. Всё созданное тестом удаляется.
func TestOwnerReleaseIntegration(t *testing.T) {
	dsn := os.Getenv("OWNER_TEST_DSN")
	if dsn == "" {
		t.Skip("OWNER_TEST_DSN не задан — интеграционный тест ответственных пропущен")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cleanup := func() {
		_, _ = db.Exec(`DELETE FROM deals WHERE client_id IN (SELECT id FROM clients WHERE name = 'owner-it-client')`)
		_, _ = db.Exec(`DELETE FROM clients WHERE name = 'owner-it-client'`)
		_, _ = db.Exec(`DELETE FROM leads WHERE title LIKE 'owner-it-%'`)
		_, _ = db.Exec(`DELETE FROM users WHERE email LIKE 'owner-it-%@test.local'`)
	}
	cleanup()
	defer cleanup()

	newUser := func(email string) int {
		var id int
		if err := db.QueryRow(`INSERT INTO users (email, role_id, password_hash, is_active, is_verified)
			VALUES ($1, 10, 'x', TRUE, TRUE) RETURNING id`, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	gone := newUser("owner-it-gone@test.local")
	peer := newUser("owner-it-peer@test.local")
	other := newUser("owner-it-other@test.local")

	var clientID int
	if err := db.QueryRow(`INSERT INTO clients (name, owner_id, client_type) VALUES ('owner-it-client', $1, 'individual') RETURNING id`, peer).Scan(&clientID); err != nil {
		t.Fatal(err)
	}
	var leadWork, leadArch, dealWork, dealArch int
	if err := db.QueryRow(`INSERT INTO leads (title, owner_id, status) VALUES ('owner-it-work', $1, 'new') RETURNING id`, gone).Scan(&leadWork); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO leads (title, owner_id, status, is_archived) VALUES ('owner-it-arch', $1, 'new', TRUE) RETURNING id`, gone).Scan(&leadArch); err != nil {
		t.Fatal(err)
	}
	insDeal := `INSERT INTO deals (client_id, owner_id, amount, currency, status, created_at, is_archived) VALUES ($1, $2, 1, 'KZT', 'new', NOW(), $3) RETURNING id`
	if err := db.QueryRow(insDeal, clientID, gone, false).Scan(&dealWork); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(insDeal, clientID, gone, true).Scan(&dealArch); err != nil {
		t.Fatal(err)
	}

	leads := NewLeadRepository(db)
	deals := NewDealRepository(db)

	if n, _, err := leads.ReleaseOwner(gone); err != nil || n != 1 {
		t.Fatalf("lead release: n=%d err=%v (archived lead must stay)", n, err)
	}
	if n, _, err := deals.ReleaseOwner(gone); err != nil || n != 1 {
		t.Fatalf("deal release: n=%d err=%v (archived deal must stay)", n, err)
	}
	var archOwner sql.NullInt64
	_ = db.QueryRow(`SELECT owner_id FROM leads WHERE id = $1`, leadArch).Scan(&archOwner)
	if !archOwner.Valid || int(archOwner.Int64) != gone {
		t.Fatalf("archived lead must keep its owner for history, got %v", archOwner)
	}

	// Сделка без ответственного читается (COALESCE) и правится (NULLIF).
	d, err := deals.GetByID(dealWork)
	if err != nil || d == nil || d.OwnerID != 0 {
		t.Fatalf("unowned deal must read as owner 0, got %+v err=%v", d, err)
	}
	d.Amount = 2
	if err := deals.Update(d); err != nil {
		t.Fatalf("unowned deal must stay editable, got %v", err)
	}

	// «Взять себе»: первый забирает, второй — нет.
	if ok, err := leads.ClaimOwner(leadWork, peer, 0); err != nil || !ok {
		t.Fatalf("first take must win: ok=%v err=%v", ok, err)
	}
	if ok, _ := leads.ClaimOwner(leadWork, other, 0); ok {
		t.Fatal("second take of the same lead must lose")
	}
	if ok, err := deals.ClaimOwner(dealWork, other, 0); err != nil || !ok {
		t.Fatalf("deal take must win: ok=%v err=%v", ok, err)
	}
	if ok, _ := deals.ClaimOwner(dealWork, peer, 0); ok {
		t.Fatal("second take of the same deal must lose")
	}
	if l, _ := leads.GetByID(leadWork); l == nil || l.OwnerID != peer {
		t.Fatalf("lead must belong to the first taker, got %+v", l)
	}
	_ = dealArch
}
