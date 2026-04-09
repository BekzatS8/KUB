package repositories

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"turcompany/internal/models"
)

type scriptedStep struct {
	kind    string
	query   string
	args    []any
	columns []string
	rows    [][]driver.Value
	result  driver.Result
	err     error
}

type scriptedDriver struct {
	steps []scriptedStep
	pos   int32
}

func (d *scriptedDriver) Open(name string) (driver.Conn, error) {
	return &scriptedConn{driver: d}, nil
}

func (d *scriptedDriver) nextStep(kind string) (scriptedStep, error) {
	idx := int(atomic.AddInt32(&d.pos, 1) - 1)
	if idx >= len(d.steps) {
		return scriptedStep{}, fmt.Errorf("unexpected %s step at index %d", kind, idx)
	}
	step := d.steps[idx]
	if step.kind != kind {
		return scriptedStep{}, fmt.Errorf("unexpected step kind at index %d: got %s want %s", idx, step.kind, kind)
	}
	return step, nil
}

func (d *scriptedDriver) consumedAll() bool {
	return int(atomic.LoadInt32(&d.pos)) == len(d.steps)
}

type scriptedConn struct {
	driver *scriptedDriver
}

func (c *scriptedConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}
func (c *scriptedConn) Close() error { return nil }
func (c *scriptedConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *scriptedConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	step, err := c.driver.nextStep("begin")
	if err != nil {
		return nil, err
	}
	if step.err != nil {
		return nil, step.err
	}
	return &scriptedTx{driver: c.driver}, nil
}

func (c *scriptedConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	step, err := c.driver.nextStep("query")
	if err != nil {
		return nil, err
	}
	if !strings.Contains(normalizeSpace(query), normalizeSpace(step.query)) {
		return nil, fmt.Errorf("unexpected query: %q does not contain %q", query, step.query)
	}
	if err := assertArgs(args, step.args); err != nil {
		return nil, err
	}
	if step.err != nil {
		return nil, step.err
	}
	return &scriptedRows{columns: step.columns, rows: step.rows}, nil
}

func (c *scriptedConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	step, err := c.driver.nextStep("exec")
	if err != nil {
		return nil, err
	}
	if !strings.Contains(normalizeSpace(query), normalizeSpace(step.query)) {
		return nil, fmt.Errorf("unexpected exec query: %q does not contain %q", query, step.query)
	}
	if err := assertArgs(args, step.args); err != nil {
		return nil, err
	}
	if step.err != nil {
		return nil, step.err
	}
	if step.result == nil {
		return driver.RowsAffected(1), nil
	}
	return step.result, nil
}

func (c *scriptedConn) CheckNamedValue(*driver.NamedValue) error { return nil }

type scriptedTx struct {
	driver *scriptedDriver
}

func (tx *scriptedTx) Commit() error {
	step, err := tx.driver.nextStep("commit")
	if err != nil {
		return err
	}
	return step.err
}

func (tx *scriptedTx) Rollback() error {
	step, err := tx.driver.nextStep("rollback")
	if err != nil {
		return err
	}
	return step.err
}

type scriptedRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
}

func (r *scriptedRows) Columns() []string { return r.columns }
func (r *scriptedRows) Close() error      { return nil }
func (r *scriptedRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

func assertArgs(got []driver.NamedValue, expected []any) error {
	if len(got) != len(expected) {
		return fmt.Errorf("unexpected args len: got %d want %d", len(got), len(expected))
	}
	for i := range got {
		if !sameArgValue(got[i].Value, expected[i]) {
			return fmt.Errorf("unexpected arg %d: got %v want %v", i, got[i].Value, expected[i])
		}
	}
	return nil
}

func sameArgValue(a, b any) bool {
	if ai, ok := asInt64(a); ok {
		if bi, ok := asInt64(b); ok {
			return ai == bi
		}
	}
	return reflect.DeepEqual(a, b)
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int8:
		return int64(n), true
	case int16:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case uint:
		return int64(n), true
	case uint8:
		return int64(n), true
	case uint16:
		return int64(n), true
	case uint32:
		return int64(n), true
	case uint64:
		return int64(n), true
	default:
		return 0, false
	}
}

func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func TestLeadRepository_ConvertToDeal_ExistingDealDoesNotUseOuterJoinForUpdate(t *testing.T) {
	driverName := fmt.Sprintf("scripted-convert-%d", time.Now().UnixNano())
	mockDriver := &scriptedDriver{
		steps: []scriptedStep{
			{kind: "begin"},
			{
				kind:    "query",
				query:   "SELECT status FROM leads WHERE id = $1 FOR UPDATE",
				args:    []any{int64(42)},
				columns: []string{"status"},
				rows:    [][]driver.Value{{"confirmed"}},
			},
			{
				kind:  "query",
				query: "FROM deals d WHERE d.lead_id = $1 ORDER BY d.created_at DESC LIMIT 1 FOR UPDATE",
				args:  []any{int64(42)},
				columns: []string{
					"id", "lead_id", "client_id", "owner_id", "amount", "currency", "status", "created_at",
				},
				rows: [][]driver.Value{{int64(77), int64(42), int64(105), int64(9), float64(1234.56), "USD", "new", time.Date(2026, time.April, 9, 10, 0, 0, 0, time.UTC)}},
			},
			{
				kind:    "query",
				query:   "SELECT client_type FROM clients WHERE id = $1",
				args:    []any{int64(105)},
				columns: []string{"client_type"},
				rows:    [][]driver.Value{{"individual"}},
			},
			{
				kind:  "exec",
				query: "UPDATE leads SET status = 'converted' WHERE id = $1",
				args:  []any{int64(42)},
			},
			{kind: "commit"},
		},
	}
	sql.Register(driverName, mockDriver)

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	repo := NewLeadRepository(db)

	gotDeal, err := repo.ConvertToDeal(context.Background(), 42, &models.Deals{}, nil)
	if !errors.Is(err, ErrDealAlreadyExists) {
		t.Fatalf("expected ErrDealAlreadyExists, got: %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "FOR UPDATE cannot be applied to the nullable side of an outer join") {
		t.Fatalf("expected no outer join FOR UPDATE error, got: %v", err)
	}
	if gotDeal == nil {
		t.Fatal("expected existing deal, got nil")
	}
	if gotDeal.ID != 77 {
		t.Fatalf("unexpected deal id: got %d, want 77", gotDeal.ID)
	}
	if gotDeal.ClientType != "individual" {
		t.Fatalf("unexpected client type: got %q, want %q", gotDeal.ClientType, "individual")
	}
	if !mockDriver.consumedAll() {
		t.Fatalf("not all scripted steps were consumed: pos=%d total=%d", mockDriver.pos, len(mockDriver.steps))
	}
}
