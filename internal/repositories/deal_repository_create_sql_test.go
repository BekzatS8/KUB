package repositories

import (
	"os"
	"strings"
	"testing"
)

func TestDealRepositoryCreate_SQLStatementIsConsistent(t *testing.T) {
	data, err := os.ReadFile("deal_repository.go")
	if err != nil {
		t.Fatalf("read repository source: %v", err)
	}
	src := string(data)

	createInsert := "INSERT INTO deals (lead_id, client_id, owner_id, branch_id, amount, currency, status, created_at)"
	if !strings.Contains(src, createInsert) {
		t.Fatalf("create query must include created_at column")
	}
	createValues := "VALUES ($1, $2, $3, $4, $5, $6, $7, $8)"
	if !strings.Contains(src, createValues) {
		t.Fatalf("create query placeholders must include $1..$8")
	}
	if !strings.Contains(src, "deal.CreatedAt, // $8") {
		t.Fatalf("create args must pass created_at as $8")
	}
}
