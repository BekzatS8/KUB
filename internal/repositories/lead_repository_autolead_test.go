package repositories

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"testing"
)

// ── minimal stub driver: answers the fallback-owner SELECT with a fixed admin id ──

type fallbackOwnerDriver struct{}
type fallbackOwnerConn struct{}
type fallbackOwnerRows struct{ done bool }

func (fallbackOwnerDriver) Open(string) (driver.Conn, error) { return fallbackOwnerConn{}, nil }
func (fallbackOwnerConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("not implemented")
}
func (fallbackOwnerConn) Close() error              { return nil }
func (fallbackOwnerConn) Begin() (driver.Tx, error) { return nil, fmt.Errorf("not implemented") }

func (fallbackOwnerConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	q := strings.ToLower(query)
	if !strings.Contains(q, "from users") || !strings.Contains(q, "role_id") {
		return nil, fmt.Errorf("unexpected fallback query shape: %s", query)
	}
	return &fallbackOwnerRows{}, nil
}

func (r *fallbackOwnerRows) Columns() []string { return []string{"id"} }
func (r *fallbackOwnerRows) Close() error      { return nil }
func (r *fallbackOwnerRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = int64(4242) // deterministic fallback admin id
	return nil
}

func init() { sql.Register("fallbackowner_stub", fallbackOwnerDriver{}) }

// TestResolveAutoLeadOwner_PrefersResolvedManager: when a manager/integration user is
// known, it is used verbatim (no DB lookup, no fallback).
func TestResolveAutoLeadOwner_PrefersResolvedManager(t *testing.T) {
	got, err := resolveAutoLeadOwner(context.Background(), nil, 17)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 17 {
		t.Fatalf("preferred owner must be used as-is, want 17 got %d", got)
	}
}

// TestResolveAutoLeadOwner_ReturnsZeroWhenNoOwner: when no preferred owner is provided
// (preferred=0), the function returns 0 so the caller can store NULL in the DB.
// Inbound leads (Wazzup, telephony) intentionally start without a responsible owner.
func TestResolveAutoLeadOwner_ReturnsZeroWhenNoOwner(t *testing.T) {
	got, err := resolveAutoLeadOwner(context.Background(), nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Fatalf("expected 0 (no owner), got %d", got)
	}
}

// ── A2: buildLeadListWhere department fail-closed clause ──────────────────────

// TestBuildLeadListWhere_DepartmentFailClosedWithOwner: a department-scoped role sees
// its department OR its own NULL-department leads — never all NULL-department leads.
func TestBuildLeadListWhere_DepartmentFailClosedWithOwner(t *testing.T) {
	dept, owner := 2, 50
	where, args := buildLeadListWhere(LeadListFilter{DepartmentID: &dept, ScopeUserID: &owner}, 1)

	if !strings.Contains(where, "l.department_id = $1 OR (l.department_id IS NULL AND l.owner_id = $2)") {
		t.Fatalf("expected fail-closed dept+owner clause, got: %s", where)
	}
	if len(args) != 2 || args[0] != dept || args[1] != owner {
		t.Fatalf("unexpected args: %#v", args)
	}
}

// TestBuildLeadListWhere_DepartmentWithoutOwnerNoNullLeak: without a scope user the
// clause is a strict department match (no "OR department_id IS NULL" leak).
func TestBuildLeadListWhere_DepartmentWithoutOwnerNoNullLeak(t *testing.T) {
	dept := 2
	where, args := buildLeadListWhere(LeadListFilter{DepartmentID: &dept}, 1)

	if !strings.Contains(where, "l.department_id = $1") {
		t.Fatalf("expected strict department clause, got: %s", where)
	}
	if strings.Contains(where, "IS NULL") {
		t.Fatalf("department filter must NOT leak NULL-department leads, got: %s", where)
	}
	if len(args) != 1 || args[0] != dept {
		t.Fatalf("unexpected args: %#v", args)
	}
}
