package repositories

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"turcompany/internal/models"
)

// Доступ в хранилище филиалу, отделу и всем (миграция 086) на настоящем
// PostgreSQL:
//
//	DRIVE_TEST_DSN="postgres://user:pass@localhost:5432/db?sslmode=disable" go test ./internal/repositories -run DriveGroupShares
//
// Нужны таблицы users, branches, departments и миграции 082, 086. Всё
// созданное тестом удаляется.
func TestDriveGroupSharesIntegration(t *testing.T) {
	dsn := os.Getenv("DRIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("DRIVE_TEST_DSN не задан — интеграционный тест групповых доступов пропущен")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	cleanup := func() {
		_, _ = db.Exec(`DELETE FROM drive_nodes WHERE name LIKE 'drg-test-%'`)
		_, _ = db.Exec(`DELETE FROM users WHERE email LIKE 'drg-%@test.local'`)
		_, _ = db.Exec(`DELETE FROM branches WHERE code LIKE 'drg-%'`)
		_, _ = db.Exec(`DELETE FROM departments WHERE code LIKE 'drg-%'`)
	}
	cleanup()
	defer cleanup()

	var almaty, shymkent, dept int
	if err := db.QueryRow(`INSERT INTO branches (name, code) VALUES ('drg Алматы', 'drg-alm') RETURNING id`).Scan(&almaty); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO branches (name, code) VALUES ('drg Шымкент', 'drg-shm') RETURNING id`).Scan(&shymkent); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO departments (name, code) VALUES ('drg Визовый', 'drg-visa') RETURNING id`).Scan(&dept); err != nil {
		t.Fatal(err)
	}
	mkUser := func(email string, branch, department *int) int {
		var id int
		if err := db.QueryRow(`INSERT INTO users (email, first_name, last_name, role_id, password_hash, is_active, branch_id, department_id)
			VALUES ($1, 'Тест', $1, 10, 'x', TRUE, $2, $3) RETURNING id`, email, branch, department).Scan(&id); err != nil {
			t.Fatalf("create user %s: %v", email, err)
		}
		return id
	}
	admin := mkUser("drg-admin@test.local", nil, nil)
	almatyUser := mkUser("drg-alm@test.local", &almaty, nil)
	shymkentUser := mkUser("drg-shm@test.local", &shymkent, nil)
	visaUser := mkUser("drg-visa@test.local", &shymkent, &dept)
	loner := mkUser("drg-loner@test.local", nil, nil)

	repo := NewDriveRepository(db)
	mk := func(parent *int64, kind, name string) *models.DriveNode {
		n := &models.DriveNode{ParentID: parent, Kind: kind, Name: name, CreatedBy: &admin}
		if kind == models.DriveKindFile {
			n.StorageKey = "drive/test/" + name
			n.SizeBytes = 10
		}
		if err := repo.CreateNode(ctx, n); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return n
	}
	branchFolder := mk(nil, models.DriveKindFolder, "drg-test-branch")
	inBranch := mk(&branchFolder.ID, models.DriveKindFile, "drg-test-inside.pdf")
	deptFile := mk(nil, models.DriveKindFile, "drg-test-dept.pdf")
	allFile := mk(nil, models.DriveKindFile, "drg-test-all.pdf")

	can := func(node int64, user int) bool {
		t.Helper()
		ok, err := repo.CanAccess(ctx, node, user)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}

	if err := repo.UpsertGroupShares(ctx, branchFolder.ID, models.DriveShareBranch, []int{almaty}, nil, admin, "edit"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertGroupShares(ctx, deptFile.ID, models.DriveShareDepartment, []int{dept}, nil, admin, "edit"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertGroupShares(ctx, allFile.ID, models.DriveShareAll, nil, nil, admin, "edit"); err != nil {
		t.Fatal(err)
	}

	t.Run("филиал видит папку и её содержимое, другой филиал — нет", func(t *testing.T) {
		if !can(branchFolder.ID, almatyUser) || !can(inBranch.ID, almatyUser) {
			t.Fatal("branch member must access branch folder and its content")
		}
		if can(branchFolder.ID, shymkentUser) {
			t.Fatal("other branch must not access")
		}
	})

	t.Run("отдел", func(t *testing.T) {
		if !can(deptFile.ID, visaUser) || can(deptFile.ID, shymkentUser) {
			t.Fatal("only department members must access")
		}
	})

	t.Run("все", func(t *testing.T) {
		if !can(allFile.ID, loner) || !can(allFile.ID, almatyUser) {
			t.Fatal("everyone must access")
		}
	})

	t.Run("новый сотрудник филиала получает доступ сам", func(t *testing.T) {
		newcomer := mkUser("drg-new@test.local", &almaty, nil)
		if !can(branchFolder.ID, newcomer) {
			t.Fatal("newcomer of the branch must access without a new grant")
		}
		// Переведён в другой филиал — доступ пропал.
		if _, err := db.Exec(`UPDATE users SET branch_id = $1 WHERE id = $2`, shymkent, newcomer); err != nil {
			t.Fatal(err)
		}
		if can(branchFolder.ID, newcomer) {
			t.Fatal("transferred employee must lose branch access")
		}
	})

	t.Run("«Доступные мне» без дублей и с самым долгим сроком", func(t *testing.T) {
		soon := time.Now().Add(2 * time.Hour)
		if err := repo.UpsertShares(ctx, branchFolder.ID, []int{almatyUser}, &soon, admin, "edit"); err != nil {
			t.Fatal(err)
		}
		roots, err := repo.SharedRoots(ctx, almatyUser)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[int64]int{}
		for _, n := range roots {
			seen[n.ID]++
			if n.ID == branchFolder.ID && n.ShareExpiresAt != nil {
				t.Fatalf("branch grant is unlimited, the longest term must win: %v", n.ShareExpiresAt)
			}
		}
		if seen[branchFolder.ID] != 1 || seen[allFile.ID] != 1 || seen[deptFile.ID] != 0 || seen[inBranch.ID] != 0 {
			t.Fatalf("unexpected shared roots: %+v", seen)
		}
	})

	t.Run("повторная выдача группе обновляет срок, список доступов с названиями", func(t *testing.T) {
		later := time.Now().Add(48 * time.Hour)
		if err := repo.UpsertGroupShares(ctx, branchFolder.ID, models.DriveShareBranch, []int{almaty}, &later, admin, "edit"); err != nil {
			t.Fatal(err)
		}
		shares, err := repo.ListShares(ctx, branchFolder.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(shares) != 2 {
			t.Fatalf("expected branch + personal share, got %+v", shares)
		}
		if shares[0].Target != models.DriveShareBranch || shares[0].Label != "drg Алматы" || shares[0].ExpiresAt == nil || shares[0].BranchID == nil {
			t.Fatalf("branch share must come first with its name and new term: %+v", shares[0])
		}
		if shares[1].Target != models.DriveShareUser || shares[1].UserID != almatyUser {
			t.Fatalf("personal share mismatch: %+v", shares[1])
		}
		all, _ := repo.ListShares(ctx, allFile.ID)
		if len(all) != 1 || all[0].Target != models.DriveShareAll || all[0].Label != "Все сотрудники" {
			t.Fatalf("all share mismatch: %+v", all)
		}
	})

	t.Run("филиалы и отделы с числом сотрудников", func(t *testing.T) {
		branches, departments, err := repo.ListShareGroups(ctx)
		if err != nil {
			t.Fatal(err)
		}
		members := map[int]int{}
		for _, g := range branches {
			members[g.ID] = g.Members
		}
		for _, g := range departments {
			members[-g.ID] = g.Members
		}
		if members[almaty] != 1 || members[shymkent] != 3 || members[-dept] != 1 {
			t.Fatalf("unexpected member counts: %+v", members)
		}
	})
}
