package repositories

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

type leadListRegressionDriver struct{}

type leadListRegressionConn struct {
	mode string
}

type leadListRegressionRows struct {
	done bool
}

func (d *leadListRegressionDriver) Open(name string) (driver.Conn, error) {
	return &leadListRegressionConn{mode: name}, nil
}

func (c *leadListRegressionConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *leadListRegressionConn) Close() error { return nil }

func (c *leadListRegressionConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *leadListRegressionConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, 0, len(args))
	for i, arg := range args {
		named = append(named, driver.NamedValue{Ordinal: i + 1, Value: arg})
	}
	return c.QueryContext(context.Background(), query, named)
}

func (c *leadListRegressionConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	lowered := strings.ToLower(query)
	if !strings.Contains(lowered, "from leads l left join branches") {
		return nil, fmt.Errorf("unexpected query shape: %s", query)
	}

	switch c.mode {
	case "all-q-only":
		for _, part := range []string{"like $1", "limit $2 offset $3"} {
			if !strings.Contains(lowered, part) {
				return nil, fmt.Errorf("missing expected part %q in query: %s", part, query)
			}
		}
		if strings.Contains(lowered, "any(") || strings.Contains(lowered, "branch_id =") {
			return nil, fmt.Errorf("unexpected extra filter in q-only query: %s", query)
		}
		if len(args) != 3 {
			return nil, fmt.Errorf("expected 3 args for q-only filter, got %d", len(args))
		}
		if got, ok := args[0].Value.(string); !ok || got != "%smoke%" {
			return nil, fmt.Errorf("unexpected q arg: %#v", args[0].Value)
		}
	case "all":
		for _, part := range []string{"any($1)", "like $2", "l.branch_id = $3", "limit $4 offset $5"} {
			if !strings.Contains(lowered, part) {
				return nil, fmt.Errorf("missing expected part %q in query: %s", part, query)
			}
		}
		if len(args) != 5 {
			return nil, fmt.Errorf("expected 5 args for all-scope filter, got %d", len(args))
		}
		if got, ok := args[1].Value.(string); !ok || got != "%smoke%" {
			return nil, fmt.Errorf("unexpected q arg: %#v", args[1].Value)
		}
		if got, ok := toInt(args[2].Value); !ok || got != 1 {
			return nil, fmt.Errorf("unexpected branch arg: %#v", args[2].Value)
		}
	case "owner":
		for _, part := range []string{"owner_id = $1", "like $2", "l.branch_id = $3", "limit $4 offset $5"} {
			if !strings.Contains(lowered, part) {
				return nil, fmt.Errorf("missing expected part %q in query: %s", part, query)
			}
		}
		if len(args) != 5 {
			return nil, fmt.Errorf("expected 5 args for owner-scope filter, got %d", len(args))
		}
		if got, ok := toInt(args[0].Value); !ok || got != 77 {
			return nil, fmt.Errorf("unexpected owner arg: %#v", args[0].Value)
		}
		if got, ok := args[1].Value.(string); !ok || got != "%smoke%" {
			return nil, fmt.Errorf("unexpected q arg: %#v", args[1].Value)
		}
		if got, ok := toInt(args[2].Value); !ok || got != 1 {
			return nil, fmt.Errorf("unexpected branch arg: %#v", args[2].Value)
		}
	case "owner-status-group":
		for _, part := range []string{"owner_id = $1", "any($2)", "like $3", "l.branch_id = $4", "limit $5 offset $6"} {
			if !strings.Contains(lowered, part) {
				return nil, fmt.Errorf("missing expected part %q in query: %s", part, query)
			}
		}
		if len(args) != 6 {
			return nil, fmt.Errorf("expected 6 args for owner status_group filter, got %d", len(args))
		}
		if got, ok := toInt(args[0].Value); !ok || got != 77 {
			return nil, fmt.Errorf("unexpected owner arg: %#v", args[0].Value)
		}
		if got, ok := args[2].Value.(string); !ok || got != "%smoke%" {
			return nil, fmt.Errorf("unexpected q arg: %#v", args[2].Value)
		}
		if got, ok := toInt(args[3].Value); !ok || got != 1 {
			return nil, fmt.Errorf("unexpected branch arg: %#v", args[3].Value)
		}
	default:
		return nil, fmt.Errorf("unexpected regression mode: %s", c.mode)
	}

	return &leadListRegressionRows{}, nil
}

func TestListAllWithFilter_QueryOnly_UsesValidPlaceholders(t *testing.T) {
	driverName := fmt.Sprintf("lead-list-regression-q-only-%d", time.Now().UnixNano())
	sql.Register(driverName, &leadListRegressionDriver{})
	db, err := sql.Open(driverName, "all-q-only")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	repo := NewLeadRepository(db)
	filter := LeadListFilter{Query: "smoke"}

	leads, err := repo.ListAllWithFilterAndArchiveScope(5, 0, filter, ArchiveScopeActiveOnly)
	if err != nil {
		t.Fatalf("ListAllWithFilterAndArchiveScope returned error: %v", err)
	}
	if len(leads) != 1 {
		t.Fatalf("expected one row, got %d", len(leads))
	}
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}

func (r *leadListRegressionRows) Columns() []string {
	return []string{"id", "title", "description", "phone", "source", "created_at", "owner_id", "branch_id", "branch_name", "status", "is_archived", "archived_at", "archived_by", "archive_reason"}
}

func (r *leadListRegressionRows) Close() error { return nil }

func (r *leadListRegressionRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	row := []driver.Value{1, "t", "d", "7700", "web", now, 77, 1, "Main", "new", false, nil, nil, ""}
	for i := range dest {
		dest[i] = row[i]
	}
	return nil
}

func TestListAllWithFilter_StatusGroupQueryBranch_UsesValidPlaceholders(t *testing.T) {
	driverName := fmt.Sprintf("lead-list-regression-all-%d", time.Now().UnixNano())
	sql.Register(driverName, &leadListRegressionDriver{})
	db, err := sql.Open(driverName, "all")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	branchID := 1
	repo := NewLeadRepository(db)
	filter := LeadListFilter{
		StatusGroup: "active",
		Query:       "smoke",
		BranchID:    &branchID,
	}

	leads, err := repo.ListAllWithFilterAndArchiveScope(5, 0, filter, ArchiveScopeActiveOnly)
	if err != nil {
		t.Fatalf("ListAllWithFilterAndArchiveScope returned error: %v", err)
	}
	if len(leads) != 1 {
		t.Fatalf("expected one row, got %d", len(leads))
	}
}

func TestListByOwnerWithFilter_QueryBranch_UsesValidPlaceholders(t *testing.T) {
	driverName := fmt.Sprintf("lead-list-regression-owner-%d", time.Now().UnixNano())
	sql.Register(driverName, &leadListRegressionDriver{})
	db, err := sql.Open(driverName, "owner")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	branchID := 1
	repo := NewLeadRepository(db)
	filter := LeadListFilter{
		Query:    "smoke",
		BranchID: &branchID,
	}

	leads, err := repo.ListByOwnerWithFilterAndArchiveScope(77, 5, 0, filter, ArchiveScopeActiveOnly)
	if err != nil {
		t.Fatalf("ListByOwnerWithFilterAndArchiveScope returned error: %v", err)
	}
	if len(leads) != 1 {
		t.Fatalf("expected one row, got %d", len(leads))
	}
}

func TestListByOwnerWithFilter_StatusGroupQueryBranch_UsesValidPlaceholders(t *testing.T) {
	driverName := fmt.Sprintf("lead-list-regression-owner-status-group-%d", time.Now().UnixNano())
	sql.Register(driverName, &leadListRegressionDriver{})
	db, err := sql.Open(driverName, "owner-status-group")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	branchID := 1
	repo := NewLeadRepository(db)
	filter := LeadListFilter{
		StatusGroup: "active",
		Query:       "smoke",
		BranchID:    &branchID,
	}

	leads, err := repo.ListByOwnerWithFilterAndArchiveScope(77, 5, 0, filter, ArchiveScopeActiveOnly)
	if err != nil {
		t.Fatalf("ListByOwnerWithFilterAndArchiveScope returned error: %v", err)
	}
	if len(leads) != 1 {
		t.Fatalf("expected one row, got %d", len(leads))
	}
}
