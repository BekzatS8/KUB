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

	createInsert := "INSERT INTO deals (lead_id, client_id, owner_id, branch_id, amount, currency, status, created_at, department_id)"
	if !strings.Contains(src, createInsert) {
		t.Fatalf("create query must include department_id column")
	}
	if !strings.Contains(src, "deal.CreatedAt, // $8") {
		t.Fatalf("create args must pass created_at as $8")
	}
	if !strings.Contains(src, "COALESCE") {
		t.Fatalf("create query must populate department_id via COALESCE subquery")
	}
}
