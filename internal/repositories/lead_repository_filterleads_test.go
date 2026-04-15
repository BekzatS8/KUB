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

type leadFilterCheckDriver struct{}

type leadFilterCheckConn struct{}

type leadFilterCheckRows struct{ done bool }

func (d *leadFilterCheckDriver) Open(string) (driver.Conn, error) { return &leadFilterCheckConn{}, nil }

func (c *leadFilterCheckConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("not implemented")
}
func (c *leadFilterCheckConn) Close() error              { return nil }
func (c *leadFilterCheckConn) Begin() (driver.Tx, error) { return nil, fmt.Errorf("not implemented") }

func (c *leadFilterCheckConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if strings.Count(strings.ToLower(query), "from leads") > 1 {
		return nil, fmt.Errorf("duplicated FROM leads in query: %s", query)
	}
	if !strings.Contains(strings.ToLower(query), "from leads l left join branches") {
		return nil, fmt.Errorf("unexpected query shape: %s", query)
	}
	return &leadFilterCheckRows{}, nil
}

func (c *leadFilterCheckConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, 0, len(args))
	for i, arg := range args {
		named = append(named, driver.NamedValue{Ordinal: i + 1, Value: arg})
	}
	return c.QueryContext(context.Background(), query, named)
}

func (r *leadFilterCheckRows) Columns() []string {
	return []string{"id", "title", "description", "phone", "source", "created_at", "owner_id", "branch_id", "branch_name", "status", "is_archived", "archived_at", "archived_by", "archive_reason"}
}
func (r *leadFilterCheckRows) Close() error { return nil }
func (r *leadFilterCheckRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	now := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	row := []driver.Value{1, "t", "d", "7700", "web", now, 10, 20, "Main", "new", false, nil, nil, ""}
	for i := range dest {
		dest[i] = row[i]
	}
	return nil
}

func TestFilterLeads_UsesValidFromClause(t *testing.T) {
	driverName := fmt.Sprintf("lead-filter-check-%d", time.Now().UnixNano())
	sql.Register(driverName, &leadFilterCheckDriver{})
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	repo := NewLeadRepository(db)
	leads, err := repo.FilterLeads("", 0, "created_at", "desc", 10, 0)
	if err != nil {
		t.Fatalf("FilterLeads returned error: %v", err)
	}
	if len(leads) != 1 {
		t.Fatalf("expected one row, got %d", len(leads))
	}
}
